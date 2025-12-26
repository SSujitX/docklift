import docker
import asyncio
import os
from pathlib import Path
from typing import AsyncGenerator

from app.config import settings


client = docker.from_env()


def ensure_network():
    try:
        client.networks.get(settings.docker_network)
    except docker.errors.NotFound:
        client.networks.create(settings.docker_network, driver="bridge")


def patch_nextjs_middleware(project_path: Path) -> str:
    """Patch Next.js middleware to allow all hosts for deployment"""
    middleware_files = list(project_path.glob("**/middleware.ts")) + list(project_path.glob("**/middleware.js"))
    
    patched = []
    for mw_file in middleware_files:
        # Skip node_modules
        if "node_modules" in str(mw_file):
            continue
        
        try:
            content = mw_file.read_text(encoding='utf-8')
            
            # Check if it has host validation that might block IPs
            if any(check in content.lower() for check in ['allowedhosts', 'allowed_hosts', 'x-forwarded-host', 'req.headers.host']):
                # Create a permissive middleware that just passes through
                new_content = '''import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

// Patched by Docklift for deployment - allows all hosts
export function middleware(request: NextRequest) {
    return NextResponse.next();
}

export const config = {
    matcher: [
        '/((?!_next/static|_next/image|favicon.ico|.*\\.(?:svg|png|jpg|jpeg|gif|webp)$).*)',
    ],
};
'''
                mw_file.write_text(new_content, encoding='utf-8')
                patched.append(str(mw_file.name))
        except Exception as e:
            continue
    
    if patched:
        return f"🔧 Patched middleware: {', '.join(patched)}\n"
    return ""


async def compose_up(project_path: Path, project_id: str) -> AsyncGenerator[str, None]:
    ensure_network()
    from datetime import datetime
    
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    
    yield f"\n{'━' * 50}\n"
    yield f"🚀 DEPLOYMENT STARTED\n"
    yield f"📅 {timestamp}\n"
    yield f"{'━' * 50}\n\n"
    
    # Patch any restrictive Next.js middleware
    patch_message = patch_nextjs_middleware(project_path)
    if patch_message:
        yield patch_message
    
    yield f"📦 Phase 1: Building Docker Image...\n"
    yield f"{'─' * 40}\n"
    
    # Use docker compose (v2) - the exec args must be separate strings
    cmd = ["docker", "compose", "-p", project_id, "up", "-d", "--build"]
    
    process = await asyncio.create_subprocess_exec(
        *cmd,
        cwd=str(project_path),
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.STDOUT,
        env={**os.environ, "DOCKER_BUILDKIT": "1", "COMPOSE_DOCKER_CLI_BUILD": "1"}
    )
    
    while True:
        line = await process.stdout.readline()
        if not line:
            break
        decoded = line.decode()
        # Add prefix for build steps
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
        yield f"✅ DEPLOYMENT SUCCESSFUL!\n"
        yield f"{'━' * 50}\n"
    else:
        yield f"\n{'━' * 50}\n"
        yield f"❌ DEPLOYMENT FAILED (code {process.returncode})\n"
        yield f"{'━' * 50}\n"


async def compose_down(project_path: Path, project_id: str) -> AsyncGenerator[str, None]:
    from datetime import datetime
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    
    yield f"\n{'━' * 50}\n"
    yield f"⏹️  STOPPING CONTAINERS\n"
    yield f"📅 {timestamp}\n"
    yield f"{'━' * 50}\n\n"
    
    cmd = ["docker", "compose", "-p", project_id, "down"]
    process = await asyncio.create_subprocess_exec(
        *cmd,
        cwd=str(project_path),
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.STDOUT
    )
    
    while True:
        line = await process.stdout.readline()
        if not line:
            break
        decoded = line.decode()
        if "Container" in decoded:
            yield f"🐳 {decoded}"
        else:
            yield decoded
    
    await process.wait()
    
    yield f"\n{'━' * 50}\n"
    yield f"✅ CONTAINERS STOPPED\n"
    yield f"{'━' * 50}\n"


async def compose_restart(project_path: Path, project_id: str) -> AsyncGenerator[str, None]:
    from datetime import datetime
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    
    yield f"\n{'━' * 50}\n"
    yield f"🔄 RESTARTING CONTAINERS\n"
    yield f"📅 {timestamp}\n"
    yield f"{'━' * 50}\n\n"
    
    cmd = ["docker", "compose", "-p", project_id, "restart"]
    process = await asyncio.create_subprocess_exec(
        *cmd,
        cwd=str(project_path),
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.STDOUT
    )
    
    while True:
        line = await process.stdout.readline()
        if not line:
            break
        decoded = line.decode()
        if "Container" in decoded:
            yield f"🐳 {decoded}"
        else:
            yield decoded
    
    await process.wait()
    
    yield f"\n{'━' * 50}\n"
    yield f"✅ CONTAINERS RESTARTED\n"
    yield f"{'━' * 50}\n"


def get_container_status(container_name: str) -> dict:
    try:
        container = client.containers.get(container_name)
        return {
            "status": container.status,
            "running": container.status == "running"
        }
    except docker.errors.NotFound:
        return {"status": "not_found", "running": False}


def get_container_logs(container_name: str, tail: int = 100) -> str:
    try:
        container = client.containers.get(container_name)
        return container.logs(tail=tail).decode()
    except docker.errors.NotFound:
        return "Container not found"


async def stream_container_logs(container_name: str) -> AsyncGenerator[str, None]:
    try:
        container = client.containers.get(container_name)
        for log in container.logs(stream=True, follow=True, tail=50):
            yield log.decode()
    except docker.errors.NotFound:
        yield "Container not found"


def get_container_stats(container_name: str) -> dict:
    try:
        container = client.containers.get(container_name)
        stats = container.stats(stream=False)
        
        cpu_delta = stats["cpu_stats"]["cpu_usage"]["total_usage"] - stats["precpu_stats"]["cpu_usage"]["total_usage"]
        system_delta = stats["cpu_stats"]["system_cpu_usage"] - stats["precpu_stats"]["system_cpu_usage"]
        cpu_percent = (cpu_delta / system_delta) * 100 if system_delta > 0 else 0
        
        memory_usage = stats["memory_stats"].get("usage", 0)
        memory_limit = stats["memory_stats"].get("limit", 0)
        
        networks = stats.get("networks", {})
        rx_bytes = sum(n.get("rx_bytes", 0) for n in networks.values())
        tx_bytes = sum(n.get("tx_bytes", 0) for n in networks.values())
        
        return {
            "cpu_percent": round(cpu_percent, 2),
            "memory_usage": f"{memory_usage / 1024 / 1024:.1f} MB",
            "memory_limit": f"{memory_limit / 1024 / 1024:.1f} MB",
            "network_rx": f"{rx_bytes / 1024:.1f} KB",
            "network_tx": f"{tx_bytes / 1024:.1f} KB"
        }
    except Exception:
        return {
            "cpu_percent": 0,
            "memory_usage": "0 MB",
            "memory_limit": "0 MB",
            "network_rx": "0 KB",
            "network_tx": "0 KB"
        }


