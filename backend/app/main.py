from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from contextlib import asynccontextmanager

from app.database import init_db
from app.routes import projects, deployments, files, ports
from app.services.port_manager import init_ports


@asynccontextmanager
async def lifespan(app: FastAPI):
    await init_db()
    await init_ports()
    yield


app = FastAPI(
    title="App-Hostify",
    description="Self-hosted deployment platform",
    version="0.1.0",
    lifespan=lifespan
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

app.include_router(projects.router, prefix="/api/projects", tags=["projects"])
app.include_router(deployments.router, prefix="/api/deployments", tags=["deployments"])
app.include_router(files.router, prefix="/api/files", tags=["files"])
app.include_router(ports.router, prefix="/api/ports", tags=["ports"])


@app.get("/api/health")
async def health_check():
    return {"status": "healthy", "version": "0.1.0"}


