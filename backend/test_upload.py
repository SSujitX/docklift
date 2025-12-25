import requests
import zipfile
import io
import os

# Create a dummy zip file
def create_dummy_zip():
    buffer = io.BytesIO()
    with zipfile.ZipFile(buffer, 'w') as zf:
        zf.writestr('Dockerfile', 'FROM python:3.9\nCMD ["python", "--version"]')
        zf.writestr('app/main.py', 'print("Hello World")')
    buffer.seek(0)
    return buffer

def test_upload():
    url = "http://127.0.0.1:8000/api/projects"
    zip_buffer = create_dummy_zip()
    
    files = {
        'files': ('test.zip', zip_buffer, 'application/zip')
    }
    
    data = {
        'name': 'test-project-123',
        'source_type': 'upload',
        'domain': 'test.local'
    }
    
    try:
        response = requests.post(url, data=data, files=files, timeout=5)
        print(f"Status Code: {response.status_code}")
        print(f"Response: {response.text}")
    except Exception as e:
        print(f"Error: {e}")

if __name__ == "__main__":
    test_upload()
