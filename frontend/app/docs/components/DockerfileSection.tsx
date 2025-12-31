import { FileCode } from "lucide-react";
import { StaticCodeBlock } from "./DocsShared";

export const DockerfileSection = () => (
  <section id="dockerfile" className="scroll-mt-20 mb-12 text-left">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
      <FileCode className="h-6 w-6 text-cyan-500" />
      Dockerfile Requirements
    </h2>
    <p className="text-muted-foreground mb-4">
      Your project must include at least one Dockerfile. Docklift reads the <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">EXPOSE</code> directive to set up port mapping.
    </p>
    
    <div className="space-y-6">
      <StaticCodeBlock 
        title="Example: Node.js Application" 
        code={`FROM node:20-alpine
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
EXPOSE 3000
CMD ["npm", "start"]`} 
      />
      <StaticCodeBlock 
        title="Example: Python Flask" 
        code={`FROM python:3.11-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install -r requirements.txt
COPY . .
EXPOSE 5000
CMD ["python", "app.py"]`} 
      />
    </div>

    <div className="bg-gradient-to-r from-cyan-500/10 to-blue-500/10 rounded-xl p-4 border border-cyan-500/20 mt-6">
      <p className="text-sm">
        <span className="font-semibold text-cyan-500">Important:</span>{" "}
        Always include <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">EXPOSE port</code> in your Dockerfile. Docklift uses this to auto-configure port mapping.
      </p>
    </div>
  </section>
);
