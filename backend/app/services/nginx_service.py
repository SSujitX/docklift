from pathlib import Path
import asyncio

from app.config import settings


NGINX_TEMPLATE = """server {{
    listen 80;
    server_name {domain};

    location / {{
        proxy_pass http://{container_name}:{port};
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
        
        proxy_connect_timeout 600;
        proxy_send_timeout 600;
        proxy_read_timeout 600;
        send_timeout 600;
        client_max_body_size 100M;
    }}
}}
"""


async def generate_nginx_config(project_id: str, domain: str, container_name: str, port: int):
    config = NGINX_TEMPLATE.format(
        domain=domain,
        container_name=container_name,
        port=port
    )
    
    config_path = settings.nginx_conf_path / f"{project_id}.conf"
    config_path.parent.mkdir(parents=True, exist_ok=True)
    config_path.write_text(config)
    
    return config_path


async def remove_nginx_config(project_id: str):
    config_path = settings.nginx_conf_path / f"{project_id}.conf"
    if config_path.exists():
        config_path.unlink()


async def reload_nginx():
    process = await asyncio.create_subprocess_exec(
        "docker", "exec", "hostify-nginx", "nginx", "-s", "reload",
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE
    )
    await process.wait()


