from fastapi import APIRouter, Depends, HTTPException, UploadFile, File, Form
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select
from pathlib import Path
import shutil
import aiofiles
import zipfile
import tempfile
import uuid
import os

from app.database import get_db
from app.models.project import Project, ProjectStatus, SourceType
from app.schemas import ProjectCreate, ProjectResponse, ProjectUpdate
from app.services.port_manager import allocate_port, release_port
from app.services.git_service import clone_repo
from app.config import settings

router = APIRouter()


@router.get("", response_model=list[ProjectResponse])
async def list_projects(db: AsyncSession = Depends(get_db)):
    from app.services import docker_service
    
    result = await db.execute(select(Project).order_by(Project.created_at.desc()))
    projects = result.scalars().all()
    
    # Sync status with Docker for all projects
    updated = False
    for project in projects:
        if project.container_name:
            container_status = docker_service.get_container_status(project.container_name)
            if container_status["running"] and project.status not in [ProjectStatus.RUNNING, ProjectStatus.BUILDING]:
                project.status = ProjectStatus.RUNNING
                updated = True
            elif not container_status["running"] and container_status["status"] != "not_found" and project.status == ProjectStatus.RUNNING:
                project.status = ProjectStatus.STOPPED
                updated = True
    
    if updated:
        await db.commit()
    
    return projects


@router.get("/{project_id}", response_model=ProjectResponse)
async def get_project(project_id: str, db: AsyncSession = Depends(get_db)):
    from app.services import docker_service
    
    result = await db.execute(select(Project).where(Project.id == project_id))
    project = result.scalar_one_or_none()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    
    # Sync status with Docker container state
    if project.container_name:
        container_status = docker_service.get_container_status(project.container_name)
        if container_status["running"] and project.status not in [ProjectStatus.RUNNING, ProjectStatus.BUILDING]:
            project.status = ProjectStatus.RUNNING
            await db.commit()
        elif not container_status["running"] and container_status["status"] != "not_found" and project.status == ProjectStatus.RUNNING:
            project.status = ProjectStatus.STOPPED
            await db.commit()
    
    return project


@router.post("", response_model=ProjectResponse)
async def create_project(
    name: str = Form(...),
    description: str = Form(None),
    source_type: str = Form("upload"),
    github_url: str = Form(None),
    github_branch: str = Form("main"),
    domain: str = Form(None),
    files: list[UploadFile] = File(None),
    db: AsyncSession = Depends(get_db)
):
    project_id = str(uuid.uuid4())
    project = Project(
        id=project_id,
        name=name,
        description=description,
        source_type=SourceType(source_type),
        github_url=github_url,
        github_branch=github_branch,
        domain=domain,
        status=ProjectStatus.PENDING
    )
    
    port = await allocate_port(db, project.id)
    project.port = port
    project.container_name = f"hostify_{project.id[:8]}"
    
    db.add(project)
    await db.commit()
    await db.refresh(project)
    
    project_path = settings.deployments_path / project.id
    project_path.mkdir(parents=True, exist_ok=True)
    
    if source_type == "github" and github_url:
        async for _ in clone_repo(github_url, github_branch, project_path):
            pass
    elif files:
        try:
            for file in files:
                if file.filename.lower().endswith('.zip'):
                    # Use temp file for large zip uploads - more memory efficient
                    with tempfile.NamedTemporaryFile(delete=False, suffix='.zip') as tmp:
                        tmp_path = tmp.name
                        # Read in chunks for better performance with large files
                        while chunk := await file.read(1024 * 1024):  # 1MB chunks
                            tmp.write(chunk)
                    
                    try:
                        with zipfile.ZipFile(tmp_path, 'r') as zf:
                            zf.extractall(project_path)
                    finally:
                        # Clean up temp file
                        os.unlink(tmp_path)
                    
                    # Handle nested folder structure
                    items = list(project_path.iterdir())
                    if len(items) == 1 and items[0].is_dir():
                        nested_folder = items[0]
                        for item in nested_folder.iterdir():
                            # Fix: shutil.move can fail on Windows if destination exists
                            dest_path = project_path / item.name
                            if dest_path.exists():
                                if dest_path.is_dir():
                                    shutil.rmtree(dest_path)
                                else:
                                    dest_path.unlink()
                            shutil.move(str(item), str(project_path))
                        nested_folder.rmdir()
                else:
                    file_path = project_path / file.filename
                    # Write in chunks for better memory handling
                    async with aiofiles.open(file_path, 'wb') as f:
                        while chunk := await file.read(1024 * 1024):  # 1MB chunks
                            await f.write(chunk)
        except Exception as e:
            import traceback
            traceback.print_exc()
            # Clean up on failure
            if project_path.exists():
                shutil.rmtree(project_path)
            await db.delete(project)
            await db.commit()
            raise HTTPException(status_code=500, detail=str(e))
    
    return project


@router.patch("/{project_id}", response_model=ProjectResponse)
async def update_project(
    project_id: str,
    update: ProjectUpdate,
    db: AsyncSession = Depends(get_db)
):
    result = await db.execute(select(Project).where(Project.id == project_id))
    project = result.scalar_one_or_none()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    
    update_data = update.model_dump(exclude_unset=True)
    for key, value in update_data.items():
        setattr(project, key, value)
    
    await db.commit()
    await db.refresh(project)
    
    return project


@router.delete("/{project_id}")
async def delete_project(project_id: str, db: AsyncSession = Depends(get_db)):
    import asyncio
    
    result = await db.execute(select(Project).where(Project.id == project_id))
    project = result.scalar_one_or_none()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    
    project_path = settings.deployments_path / project_id
    
    # Stop and remove Docker containers
    try:
        process = await asyncio.create_subprocess_exec(
            "docker", "compose", "-p", project_id, "down", "--rmi", "all", "--volumes", "--remove-orphans",
            cwd=str(project_path) if project_path.exists() else None,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE
        )
        await process.wait()
    except Exception as e:
        print(f"Docker cleanup warning: {e}")
    
    # Also try to remove any dangling images with the project ID
    try:
        process = await asyncio.create_subprocess_exec(
            "docker", "image", "prune", "-f", "--filter", f"label=com.docker.compose.project={project_id}",
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE
        )
        await process.wait()
    except Exception:
        pass
    
    # Delete project files
    if project_path.exists():
        shutil.rmtree(project_path)
    
    await release_port(db, project_id)
    
    await db.delete(project)
    await db.commit()
    
    return {"status": "deleted", "docker_cleaned": True}


@router.post("/{project_id}/reset")
async def reset_project(project_id: str, db: AsyncSession = Depends(get_db)):
    """Reset a stuck project status to PENDING"""
    result = await db.execute(select(Project).where(Project.id == project_id))
    project = result.scalar_one_or_none()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    
    project.status = ProjectStatus.PENDING
    await db.commit()
    await db.refresh(project)
    
    return {"status": "reset", "new_status": project.status.value}
