from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import async_session
from app.models.port import PortAllocation
from app.config import settings


async def init_ports():
    async with async_session() as session:
        result = await session.execute(select(PortAllocation))
        existing = result.scalars().all()
        
        if not existing:
            for port in range(settings.port_range_start, settings.port_range_end):
                session.add(PortAllocation(port=port, is_locked=False))
            await session.commit()


async def allocate_port(session: AsyncSession, project_id: str) -> int:
    result = await session.execute(
        select(PortAllocation)
        .where(PortAllocation.is_locked == False)
        .where(PortAllocation.project_id == None)
        .order_by(PortAllocation.port)
        .limit(1)
    )
    free_port = result.scalar_one_or_none()
    
    if not free_port:
        raise Exception("No available ports in range")
    
    free_port.project_id = project_id
    free_port.is_locked = True
    await session.commit()
    
    return free_port.port


async def release_port(session: AsyncSession, project_id: str):
    result = await session.execute(
        select(PortAllocation).where(PortAllocation.project_id == project_id)
    )
    port = result.scalar_one_or_none()
    
    if port:
        port.project_id = None
        port.is_locked = False
        await session.commit()


async def get_all_ports(session: AsyncSession):
    result = await session.execute(
        select(PortAllocation).order_by(PortAllocation.port)
    )
    return result.scalars().all()


