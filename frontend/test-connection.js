const API_URL = "http://127.0.0.1:8000";

console.log("Testing connection to:", API_URL);

fetch(`${API_URL}/api/health`)
  .then((res) => res.json())
  .then((data) => console.log("Success:", data))
  .catch((err) => console.error("Error:", err));
