from fastapi import APIRouter, Depends, HTTPException, WebSocket
from fastapi.responses import StreamingResponse
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select
from datetime import datetime
import asyncio

from app.database import get_db
from app.models.project import Project, Deployment, ProjectStatus, DeploymentStatus
from app.schemas import DeploymentResponse
from app.services import docker_service
from app.services.git_service import pull_repo
from app.services.compose_service import generate_compose, check_compose_exists, detect_internal_port
from app.config import settings

router = APIRouter()


@router.get("/{project_id}", response_model=list[DeploymentResponse])
async def list_deployments(project_id: str, db: AsyncSession = Depends(get_db)):
    result = await db.execute(
        select(Deployment)
        .where(Deployment.project_id == project_id)
        .order_by(Deployment.created_at.desc())
        .limit(20)
    )
    return result.scalars().all()


@router.post("/{project_id}/deploy")
async def deploy_project(project_id: str, db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(Project).where(Project.id == project_id))
    project = result.scalar_one_or_none()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    
    project_path = settings.deployments_path / project_id
    
    if not project_path.exists():
        raise HTTPException(status_code=400, detail="Project files not found")
    
    # Check if Dockerfile exists
    if not (project_path / "Dockerfile").exists():
        raise HTTPException(status_code=400, detail="No Dockerfile found in project")
    
    deployment = Deployment(project_id=project_id, status=DeploymentStatus.IN_PROGRESS)
    db.add(deployment)
    
    project.status = ProjectStatus.BUILDING
    await db.commit()
    await db.refresh(deployment)
    deployment_id = deployment.id
    
    async def stream_deploy():
        logs = []
        success = False
        
        try:
            # Generate docker-compose.yml if not exists
            if not check_compose_exists(project_path):
                internal_port = detect_internal_port(project_path)
                yield f"📦 Generating docker-compose.yml (port {project.port} → {internal_port})...\n"
                generate_compose(project_id, project_path, project.container_name, internal_port, project.port)
                yield "✅ docker-compose.yml created\n\n"
            
            if project.source_type.value == "github" and project.github_url:
                async for line in pull_repo(project_path):
                    logs.append(line)
                    yield line
            
            async for line in docker_service.compose_up(project_path, project_id):
                logs.append(line)
                yield line
                if "Deployment successful" in line:
                    success = True
        except Exception as e:
            error_msg = f"\n❌ Error: {str(e)}\n"
            logs.append(error_msg)
            yield error_msg
        
        # Update deployment status in database using fresh queries
        try:
            # Re-fetch objects to avoid stale state
            result = await db.execute(select(Deployment).where(Deployment.id == deployment_id))
            dep = result.scalar_one_or_none()
            if dep:
                dep.logs = "".join(logs)
                dep.status = DeploymentStatus.SUCCESS if success else DeploymentStatus.FAILED
                dep.finished_at = datetime.utcnow()
            
            result = await db.execute(select(Project).where(Project.id == project_id))
            proj = result.scalar_one_or_none()
            if proj:
                proj.status = ProjectStatus.RUNNING if success else ProjectStatus.ERROR
            
            await db.commit()
        except Exception as e:
            yield f"\n⚠️ Warning: Could not update status: {str(e)}\n"
    
    return StreamingResponse(stream_deploy(), media_type="text/plain")


@router.post("/{project_id}/stop")
async def stop_project(project_id: str, db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(Project).where(Project.id == project_id))
    project = result.scalar_one_or_none()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    
    project_path = settings.deployments_path / project_id
    
    async def stream_stop():
        async for line in docker_service.compose_down(project_path, project_id):
            yield line
        
        project.status = ProjectStatus.STOPPED
        await db.commit()
    
    return StreamingResponse(stream_stop(), media_type="text/plain")


@router.post("/{project_id}/cancel")
async def cancel_build(project_id: str, db: AsyncSession = Depends(get_db)):
    """Cancel a building deployment and reset status"""
    result = await db.execute(select(Project).where(Project.id == project_id))
    project = result.scalar_one_or_none()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    
    project_path = settings.deployments_path / project_id
    
    async def stream_cancel():
        yield "❌ Cancelling build...\n"
        
        # Try to stop any running containers
        async for line in docker_service.compose_down(project_path, project_id):
            yield line
        
        project.status = ProjectStatus.STOPPED
        await db.commit()
        
        yield "✅ Build cancelled\n"
    
    return StreamingResponse(stream_cancel(), media_type="text/plain")

@router.post("/{project_id}/redeploy")
async def redeploy_project(project_id: str, db: AsyncSession = Depends(get_db)):
    """Redeploy - rebuild container with --force-recreate"""
    result = await db.execute(select(Project).where(Project.id == project_id))
    project = result.scalar_one_or_none()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    
    project_path = settings.deployments_path / project_id
    
    async def stream_redeploy():
        from datetime import datetime
        timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        
        yield f"\n{'━' * 50}\n"
        yield f"🔄 REDEPLOYING CONTAINER\n"
        yield f"📅 {timestamp}\n"
        yield f"{'━' * 50}\n\n"
        
        yield f"📦 Rebuilding with --force-recreate...\n"
        yield f"{'─' * 40}\n"
        
        process = await asyncio.create_subprocess_exec(
            "docker", "compose", "-p", project_id, "up", "-d", "--build", "--force-recreate",
            cwd=str(project_path),
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.STDOUT
        )
        
        while True:
            line = await process.stdout.readline()
            if not line:
                break
            decoded = line.decode()
            if decoded.strip().startswith("#"):
                yield f"   {decoded}"
            elif "Container" in decoded:
                yield f"\n🐳 {decoded}"
            elif "Built" in decoded:
                yield f"✅ {decoded}\n"
            else:
                yield decoded
        
        await process.wait()
        
        yield f"\n{'─' * 40}\n"
        
        if process.returncode == 0:
            yield f"\n{'━' * 50}\n"
            yield f"✅ REDEPLOY SUCCESSFUL!\n"
            yield f"{'━' * 50}\n"
        else:
            yield f"\n{'━' * 50}\n"
            yield f"❌ REDEPLOY FAILED (code {process.returncode})\n"
            yield f"{'━' * 50}\n"
    
    return StreamingResponse(stream_redeploy(), media_type="text/plain")


@router.post("/{project_id}/restart")
async def restart_project(project_id: str, db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(Project).where(Project.id == project_id))
    project = result.scalar_one_or_none()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    
    project_path = settings.deployments_path / project_id
    
    async def stream_restart():
        async for line in docker_service.compose_restart(project_path, project_id):
            yield line
    
    return StreamingResponse(stream_restart(), media_type="text/plain")


@router.get("/{project_id}/logs")
async def get_logs(project_id: str, db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(Project).where(Project.id == project_id))
    project = result.scalar_one_or_none()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    
    logs = docker_service.get_container_logs(project.container_name)
    return {"logs": logs}


@router.get("/{project_id}/stats")
async def get_stats(project_id: str, db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(Project).where(Project.id == project_id))
    project = result.scalar_one_or_none()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    
    stats = docker_service.get_container_stats(project.container_name)
    return stats


@router.websocket("/{project_id}/logs/stream")
async def stream_logs(websocket: WebSocket, project_id: str):
    await websocket.accept()
    
    async with websocket.app.state.db() as db:
        result = await db.execute(select(Project).where(Project.id == project_id))
        project = result.scalar_one_or_none()
        
        if not project:
            await websocket.close()
            return
        
        try:
            async for log in docker_service.stream_container_logs(project.container_name):
                await websocket.send_text(log)
        except Exception:
            await websocket.close()


