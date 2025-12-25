"use client";

import { Header } from "@/components/Header";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Server, Network, Container, Info } from "lucide-react";

export default function SettingsPage() {
  return (
    <div className="min-h-screen">
      <Header />

      <main className="container max-w-3xl mx-auto px-4 md:px-6 py-8">
        <div className="mb-8">
          <h1 className="text-3xl font-bold tracking-tight">Settings</h1>
          <p className="text-muted-foreground mt-1">Server configuration and preferences</p>
        </div>

        <div className="space-y-6">
          <Card className="p-6">
            <div className="flex items-center gap-3 mb-5">
              <div className="p-2.5 rounded-xl bg-primary/10">
                <Server className="h-5 w-5 text-primary" />
              </div>
              <div>
                <h2 className="font-semibold">Server</h2>
                <p className="text-sm text-muted-foreground">API and deployment settings</p>
              </div>
            </div>
            <div className="grid gap-4">
              <div className="space-y-2">
                <label className="text-sm font-medium">API URL</label>
                <Input value="http://localhost:8000" disabled className="bg-secondary/50" />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">Deployments Path</label>
                <Input value="/deployments" disabled className="bg-secondary/50" />
              </div>
            </div>
          </Card>

          <Card className="p-6">
            <div className="flex items-center gap-3 mb-5">
              <div className="p-2.5 rounded-xl bg-emerald-500/10">
                <Network className="h-5 w-5 text-emerald-500" />
              </div>
              <div>
                <h2 className="font-semibold">Port Configuration</h2>
                <p className="text-sm text-muted-foreground">Allocatable port range</p>
              </div>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <label className="text-sm font-medium">Start Port</label>
                <Input type="number" value="3001" disabled className="bg-secondary/50" />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">End Port</label>
                <Input type="number" value="3100" disabled className="bg-secondary/50" />
              </div>
            </div>
          </Card>

          <Card className="p-6">
            <div className="flex items-center gap-3 mb-5">
              <div className="p-2.5 rounded-xl bg-blue-500/10">
                <Container className="h-5 w-5 text-blue-500" />
              </div>
              <div>
                <h2 className="font-semibold">Docker</h2>
                <p className="text-sm text-muted-foreground">Container network settings</p>
              </div>
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">Network Name</label>
              <Input value="hostify_network" disabled className="bg-secondary/50" />
            </div>
          </Card>

          <Card className="p-6 bg-gradient-to-r from-violet-500/10 to-purple-500/10 border-primary/20">
            <div className="flex items-center gap-3">
              <div className="p-2.5 rounded-xl bg-primary/10">
                <Info className="h-5 w-5 text-primary" />
              </div>
              <div>
                <h2 className="font-semibold">About Hostify</h2>
                <p className="text-sm text-muted-foreground">Version 0.1.0 · Self-hosted deployment platform</p>
              </div>
            </div>
          </Card>
        </div>
      </main>
    </div>
  );
}

