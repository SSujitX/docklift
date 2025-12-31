import { Activity, Cpu, Database, HardDrive, Network, Server } from "lucide-react";

export const SystemOverview = () => (
  <section id="system" className="scroll-mt-20 mb-12 text-left">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
      <Activity className="h-6 w-6 text-cyan-500" />
      System Overview
    </h2>
    <p className="text-muted-foreground mb-4">
      Real-time monitoring of your server and Docker infrastructure.
    </p>

    <div className="bg-secondary/50 rounded-xl p-6 mb-6">
      <h4 className="font-semibold mb-4 text-emerald-500">Real-time Metrics</h4>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="flex items-center gap-3 p-3 bg-background/50 rounded-lg">
          <Cpu className="h-5 w-5 text-cyan-500" />
          <div>
            <p className="font-medium">CPU Usage</p>
            <p className="text-xs text-muted-foreground">Load average, usage per core</p>
          </div>
        </div>
        <div className="flex items-center gap-3 p-3 bg-background/50 rounded-lg">
          <Database className="h-5 w-5 text-emerald-500" />
          <div>
            <p className="font-medium">Memory (RAM)</p>
            <p className="text-xs text-muted-foreground">Used, free, cache, and swap</p>
          </div>
        </div>
        <div className="flex items-center gap-3 p-3 bg-background/50 rounded-lg">
          <HardDrive className="h-5 w-5 text-amber-500" />
          <div>
            <p className="font-medium">Disk Usage</p>
            <p className="text-xs text-muted-foreground">Read/write speed, capacity</p>
          </div>
        </div>
        <div className="flex items-center gap-3 p-3 bg-background/50 rounded-lg">
          <Network className="h-5 w-5 text-blue-500" />
          <div>
            <p className="font-medium">Network Traffic</p>
            <p className="text-xs text-muted-foreground">Upload and download speeds</p>
          </div>
        </div>
      </div>
    </div>

    <div className="bg-secondary/50 rounded-xl p-6">
      <h4 className="font-semibold mb-4">Control Plane</h4>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div className="flex items-center gap-3 p-3 bg-background/50 rounded-lg">
          <Server className="h-5 w-5 text-red-500" />
          <div>
            <p className="font-medium">Reboot</p>
            <p className="text-xs text-muted-foreground">Restart the server</p>
          </div>
        </div>
        <div className="flex items-center gap-3 p-3 bg-background/50 rounded-lg">
          <RefreshCw className="h-5 w-5 text-cyan-500" />
          <div>
            <p className="font-medium">Reset</p>
            <p className="text-xs text-muted-foreground">Restart Docklift services</p>
          </div>
        </div>
      </div>
    </div>
  </section>
);

import { RefreshCw } from "lucide-react";
