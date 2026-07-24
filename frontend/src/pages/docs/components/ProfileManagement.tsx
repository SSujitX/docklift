import { User, UserCircle, ShieldCheck } from "lucide-react";

export const ProfileManagement = () => (
  <section id="profile" className="scroll-mt-20 mb-12 text-left">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
      <User className="h-6 w-6 text-cyan-500" />
      Profile Management
    </h2>
    <p className="text-muted-foreground mb-6">
      Manage your personal account information and security settings.
    </p>

    <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-3 flex items-center gap-2 text-cyan-500">
          <UserCircle className="h-5 w-5" />
          Updating Profile
        </h4>
        <p className="text-sm text-muted-foreground mb-4">
          You can change your full name and email address at any time:
        </p>
        <ul className="list-disc list-inside text-sm text-muted-foreground space-y-2">
           <li>Click your name in the top right corner</li>
           <li>Select <span className="text-foreground font-bold">Profile Settings</span></li>
           <li>Update your info and click <span className="text-foreground font-bold">Update Profile</span></li>
        </ul>
      </div>

      <div className="bg-secondary/50 rounded-xl p-6 border border-cyan-500/10">
        <h4 className="font-semibold mb-3 flex items-center gap-2 text-cyan-500">
          <ShieldCheck className="h-5 w-5" />
          Changing Password
        </h4>
        <p className="text-sm text-muted-foreground mb-4">
          For better security, we recommend changing your password regularly:
        </p>
        <ul className="list-disc list-inside text-sm text-muted-foreground space-y-2">
           <li>In <span className="text-foreground font-bold">Profile Settings</span>, scroll to Security</li>
           <li>Enter your <span className="text-foreground font-bold">Current Password</span></li>
           <li>Enter and confirm your <span className="text-foreground font-bold">New Password</span></li>
           <li>Click <span className="text-foreground font-bold">Change Password</span></li>
        </ul>
      </div>
    </div>
  </section>
);
