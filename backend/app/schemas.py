from pydantic import BaseModel
from typing import Optional
from datetime import datetime
from enum import Enum


class SourceType(str, Enum):
    UPLOAD = "upload"
    GITHUB = "github"


class ProjectStatus(str, Enum):
    PENDING = "pending"
    BUILDING = "building"
    RUNNING = "running"
    STOPPED = "stopped"
    ERROR = "error"


class DeploymentStatus(str, Enum):
    QUEUED = "queued"
    IN_PROGRESS = "in_progress"
    SUCCESS = "success"
    FAILED = "failed"


class ProjectCreate(BaseModel):
    name: str
    description: Optional[str] = None
    source_type: SourceType = SourceType.UPLOAD
    github_url: Optional[str] = None
    github_branch: str = "main"
    domain: Optional[str] = None


class ProjectUpdate(BaseModel):
    name: Optional[str] = None
    description: Optional[str] = None
    domain: Optional[str] = None
    github_branch: Optional[str] = None


class ProjectResponse(BaseModel):
    id: str
    name: str
    description: Optional[str]
    source_type: SourceType
    github_url: Optional[str]
    github_branch: str
    domain: Optional[str]
    port: Optional[int]
    status: ProjectStatus
    container_name: Optional[str]
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True


class DeploymentResponse(BaseModel):
    id: str
    project_id: str
    status: DeploymentStatus
    logs: str
    created_at: datetime
    finished_at: Optional[datetime]

    class Config:
        from_attributes = True


class PortResponse(BaseModel):
    port: int
    project_id: Optional[str]
    is_locked: bool

    class Config:
        from_attributes = True


class FileContent(BaseModel):
    filename: str
    content: str


class ContainerStats(BaseModel):
    cpu_percent: float
    memory_usage: str
    memory_limit: str
    network_rx: str
    network_tx: str


