from sqlalchemy import Column, String, Integer, DateTime, Text, ForeignKey, Enum
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func
import enum
import uuid

from app.database import Base


class ProjectStatus(str, enum.Enum):
    PENDING = "pending"
    BUILDING = "building"
    RUNNING = "running"
    STOPPED = "stopped"
    ERROR = "error"


class SourceType(str, enum.Enum):
    UPLOAD = "upload"
    GITHUB = "github"


class Project(Base):
    __tablename__ = "projects"
    
    id = Column(String, primary_key=True, default=lambda: str(uuid.uuid4()))
    name = Column(String, nullable=False)
    description = Column(Text, nullable=True)
    source_type = Column(Enum(SourceType), default=SourceType.UPLOAD)
    github_url = Column(String, nullable=True)
    github_branch = Column(String, default="main")
    domain = Column(String, nullable=True)
    port = Column(Integer, nullable=True)
    status = Column(Enum(ProjectStatus), default=ProjectStatus.PENDING)
    container_name = Column(String, nullable=True)
    created_at = Column(DateTime, server_default=func.now())
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now())
    
    deployments = relationship("Deployment", back_populates="project", cascade="all, delete-orphan")


class DeploymentStatus(str, enum.Enum):
    QUEUED = "queued"
    IN_PROGRESS = "in_progress"
    SUCCESS = "success"
    FAILED = "failed"


class Deployment(Base):
    __tablename__ = "deployments"
    
    id = Column(String, primary_key=True, default=lambda: str(uuid.uuid4()))
    project_id = Column(String, ForeignKey("projects.id"), nullable=False)
    status = Column(Enum(DeploymentStatus), default=DeploymentStatus.QUEUED)
    logs = Column(Text, default="")
    created_at = Column(DateTime, server_default=func.now())
    finished_at = Column(DateTime, nullable=True)
    
    project = relationship("Project", back_populates="deployments")


