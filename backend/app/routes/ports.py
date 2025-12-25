from fastapi import APIRouter, Depends
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import get_db
from app.services.port_manager import get_all_ports
from app.schemas import PortResponse

router = APIRouter()


@router.get("", response_model=list[PortResponse])
async def list_ports(db: AsyncSession = Depends(get_db)):
    ports = await get_all_ports(db)
    return ports


