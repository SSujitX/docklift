import { Database } from "lucide-react";

export const ManagedDatabases = () => (
  <section id="managed-databases" className="scroll-mt-20 mb-12 text-left">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
      <Database className="h-6 w-6 text-cyan-500" />
      Managed Databases
    </h2>
    <p className="text-muted-foreground mb-4">
      DockLift can run Postgres, MySQL, MariaDB, Redis, and MongoDB as first-class
      database projects — official images, named volumes, generated credentials.
      Host ports stay off by default.
    </p>

    <div className="bg-secondary/50 rounded-xl p-6 mb-4">
      <h4 className="font-semibold mb-3">Create</h4>
      <ol className="list-decimal list-inside text-sm text-muted-foreground space-y-2">
        <li>
          Open <strong>Databases → New Database</strong>, pick an engine and name.
        </li>
        <li>DockLift creates the volume, credentials, and starts a deploy (image pull).</li>
        <li>Copy the connection URL from the database Overview → Connection panel.</li>
      </ol>
    </div>

    <div className="bg-secondary/50 rounded-xl p-6 mb-4">
      <h4 className="font-semibold mb-3">Link to an app or service</h4>
      <ul className="list-disc list-inside text-sm text-muted-foreground space-y-2">
        <li>
          From the database: <strong>Linked apps</strong> → choose project and optional
          service (empty = shared env for all services).
        </li>
        <li>
          From an app: Overview → <strong>Attach database</strong>.
        </li>
        <li>
          Injects{" "}
          <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">DATABASE_URL</code>,{" "}
          <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">REDIS_URL</code>, or{" "}
          <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">MONGODB_URI</code>{" "}
          and joins the DB container to the app Docker network.
        </li>
        <li>
          <strong>Redeploy the app</strong> so running containers see the new variable.
        </li>
        <li>
          Only one database may own a given env key on the same project/service scope.
        </li>
      </ul>
    </div>

    <div className="bg-secondary/50 rounded-xl p-6 border border-amber-500/10">
      <h4 className="font-semibold mb-3 text-amber-600 dark:text-amber-400">Security notes</h4>
      <ul className="list-disc list-inside text-sm text-muted-foreground space-y-2">
        <li>Prefer linking over publishing host ports or sharing IP:port.</li>
        <li>
          Credentials are set at create time and locked in the UI/API. Recreate the
          database to rotate passwords (official images only apply many of these on
          first volume init).
        </li>
        <li>Deleting a database removes injected env from linked apps (after teardown succeeds).</li>
        <li>
          Attaching over an existing env key requires an explicit{" "}
          <strong>Overwrite</strong> confirmation.
        </li>
      </ul>
    </div>
  </section>
);
