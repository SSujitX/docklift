import asyncio
from pathlib import Path
from typing import AsyncGenerator


async def clone_repo(url: str, branch: str, target_path: Path) -> AsyncGenerator[str, None]:
    yield f"[+] Cloning {url} (branch: {branch})...\n"
    
    target_path.mkdir(parents=True, exist_ok=True)
    
    process = await asyncio.create_subprocess_exec(
        "git", "clone", "--branch", branch, "--depth", "1", url, str(target_path),
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.STDOUT
    )
    
    while True:
        line = await process.stdout.readline()
        if not line:
            break
        yield line.decode()
    
    await process.wait()
    
    if process.returncode == 0:
        yield "[✓] Repository cloned successfully\n"
    else:
        yield f"[✗] Clone failed with code {process.returncode}\n"


async def pull_repo(repo_path: Path) -> AsyncGenerator[str, None]:
    yield "[+] Pulling latest changes...\n"
    
    process = await asyncio.create_subprocess_exec(
        "git", "pull",
        cwd=str(repo_path),
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.STDOUT
    )
    
    while True:
        line = await process.stdout.readline()
        if not line:
            break
        yield line.decode()
    
    await process.wait()
    
    if process.returncode == 0:
        yield "[✓] Pull successful\n"
    else:
        yield f"[✗] Pull failed with code {process.returncode}\n"


