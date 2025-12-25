from fastapi import APIRouter, Depends, HTTPException, UploadFile, File
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select
import aiofiles
from pathlib import Path

from app.database import get_db
from app.models.project import Project
from app.schemas import FileContent
from app.config import settings

router = APIRouter()

ALLOWED_FILES = ["Dockerfile", "docker-compose.yml", "docker-compose.yaml", "nginx.conf", ".env"]


@router.get("/{project_id}")
async def list_files(project_id: str, db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(Project).where(Project.id == project_id))
    project = result.scalar_one_or_none()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    
    project_path = settings.deployments_path / project_id
    
    if not project_path.exists():
        return []
    
    def scan_directory(path: Path, relative_base: Path) -> list:
        """Recursively scan directory and return tree structure"""
        items = []
        try:
            for item in sorted(path.iterdir(), key=lambda x: (not x.is_dir(), x.name.lower())):
                relative_path = str(item.relative_to(relative_base)).replace("\\", "/")
                
                if item.is_dir():
                    # Skip common non-essential folders
                    if item.name in [".git", "__pycache__", "node_modules", ".venv", "venv", ".next"]:
                        continue
                    items.append({
                        "name": item.name,
                        "path": relative_path,
                        "type": "folder",
                        "children": scan_directory(item, relative_base)
                    })
                else:
                    items.append({
                        "name": item.name,
                        "path": relative_path,
                        "type": "file",
                        "size": item.stat().st_size,
                        "editable": item.name in ALLOWED_FILES or item.suffix in [".yml", ".yaml", ".conf", ".env", ".py", ".js", ".ts", ".json", ".md", ".txt"]
                    })
        except PermissionError:
            pass
        return items
    
    return scan_directory(project_path, project_path)


@router.get("/{project_id}/{file_path:path}")
async def get_file(project_id: str, file_path: str, db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(Project).where(Project.id == project_id))
    project = result.scalar_one_or_none()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    
    # Sanitize path to prevent directory traversal
    safe_path = Path(file_path).as_posix()
    if ".." in safe_path:
        raise HTTPException(status_code=400, detail="Invalid file path")
    
    full_path = settings.deployments_path / project_id / safe_path
    
    if not full_path.exists():
        raise HTTPException(status_code=404, detail="File not found")
    
    if not full_path.is_file():
        raise HTTPException(status_code=400, detail="Path is not a file")
    
    async with aiofiles.open(full_path, 'r', encoding='utf-8', errors='replace') as f:
        content = await f.read()
    
    return {"filename": file_path, "content": content}


@router.put("/{project_id}/{filename}")
async def update_file(
    project_id: str,
    filename: str,
    file_content: FileContent,
    db: AsyncSession = Depends(get_db)
):
    result = await db.execute(select(Project).where(Project.id == project_id))
    project = result.scalar_one_or_none()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    
    file_path = settings.deployments_path / project_id / filename
    
    async with aiofiles.open(file_path, 'w') as f:
        await f.write(file_content.content)
    
    return {"status": "updated", "filename": filename}


@router.post("/{project_id}/upload")
async def upload_files(
    project_id: str,
    files: list[UploadFile] = File(...),
    db: AsyncSession = Depends(get_db)
):
    result = await db.execute(select(Project).where(Project.id == project_id))
    project = result.scalar_one_or_none()
    
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")
    
    project_path = settings.deployments_path / project_id
    project_path.mkdir(parents=True, exist_ok=True)
    
    uploaded = []
    for file in files:
        file_path = project_path / file.filename
        async with aiofiles.open(file_path, 'wb') as f:
            content = await file.read()
            await f.write(content)
        uploaded.append(file.filename)
    
    return {"status": "uploaded", "files": uploaded}


