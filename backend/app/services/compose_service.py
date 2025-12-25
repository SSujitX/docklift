from pathlib import Path
import yaml

from app.config import settings


COMPOSE_TEMPLATE = {
    "services": {},
    "networks": {
        "docklift_network": {
            "external": True
        }
    }
}


def generate_compose(project_id: str, project_path: Path, container_name: str, internal_port: int, external_port: int) -> str:
    """Generate docker-compose.yml for a project"""
    
    compose = COMPOSE_TEMPLATE.copy()
    compose["services"] = {
        container_name: {
            "build": ".",
            "container_name": container_name,
            "restart": "unless-stopped",
            "ports": [f"{external_port}:{internal_port}"],
            "networks": ["docklift_network"],
            "environment": [
                f"PORT={internal_port}"
            ]
        }
    }
    
    compose_path = project_path / "docker-compose.yml"
    
    # Use safe_dump for proper YAML formatting
    yaml_content = yaml.dump(compose, default_flow_style=False, sort_keys=False)
    compose_path.write_text(yaml_content)
    
    return str(compose_path)


def check_compose_exists(project_path: Path) -> bool:
    """Check if docker-compose.yml already exists"""
    return (project_path / "docker-compose.yml").exists() or (project_path / "docker-compose.yaml").exists()


def detect_internal_port(project_path: Path) -> int:
    """Try to detect the internal port from Dockerfile EXPOSE directive"""
    dockerfile = project_path / "Dockerfile"
    if dockerfile.exists():
        content = dockerfile.read_text()
        for line in content.split('\n'):
            line = line.strip()
            if line.upper().startswith('EXPOSE'):
                try:
                    port = int(line.split()[1])
                    return port
                except (IndexError, ValueError):
                    pass
    return 3000  # Default fallback
