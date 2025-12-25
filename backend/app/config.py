from pydantic_settings import BaseSettings
from pathlib import Path


class Settings(BaseSettings):
    database_url: str = "sqlite+aiosqlite:///./data/docklift.db"
    deployments_path: Path = Path("./deployments")
    nginx_conf_path: Path = Path("./nginx-conf")
    port_range_start: int = 3001
    port_range_end: int = 3100
    docker_network: str = "docklift_network"
    
    class Config:
        env_prefix = "DOCKLIFT_"


settings = Settings()
