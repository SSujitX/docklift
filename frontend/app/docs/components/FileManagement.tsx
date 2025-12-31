import { FolderTree } from "lucide-react";

export const FileManagement = () => (
  <section id="files" className="scroll-mt-20 mb-12 text-left">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
      <FolderTree className="h-6 w-6 text-cyan-500" />
      File Management
    </h2>
    <p className="text-muted-foreground mb-4">
      Browse and edit project files directly in the browser.
    </p>
    <div className="bg-secondary/50 rounded-xl p-6">
      <h4 className="font-semibold mb-3">Features</h4>
      <ul className="list-disc list-inside text-muted-foreground space-y-1">
        <li>Tree view of all project files</li>
        <li>Monaco-based code editor (VS Code engine)</li>
        <li>Syntax highlighting for 100+ languages</li>
        <li>Edit Dockerfile, config files, source code</li>
        <li>Changes saved instantly to disk</li>
      </ul>
    </div>
  </section>
);
