from sqlalchemy import Column, Integer, String, Boolean

from app.database import Base


class PortAllocation(Base):
    __tablename__ = "ports"
    
    port = Column(Integer, primary_key=True)
    project_id = Column(String, nullable=True)
    is_locked = Column(Boolean, default=False)


