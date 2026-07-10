import type { Icon } from "@phosphor-icons/react";
import {
  ChartLineUp,
  FolderSimple,
  GaugeIcon,
  Key,
  ListChecks,
  PlayCircle,
  PlugsConnected,
  Pulse,
  ShieldCheck,
  StackSimple,
  Wallet,
  Wrench,
} from "@phosphor-icons/react";

export interface NavItem {
  to: string;
  label: string;
  icon: Icon;
  /** requires control_plane.enabled */
  controlPlane?: boolean;
  /** requires the P1 admin analytics endpoints */
  admin?: boolean;
  /** panel implemented and routed */
  ready?: boolean;
  group: "Observe" | "Manage" | "Tools";
}

export const NAV: NavItem[] = [
  { to: "/overview", label: "Overview", icon: GaugeIcon, group: "Observe", ready: true },
  { to: "/usage", label: "Usage & Cost", icon: ChartLineUp, group: "Observe", admin: true, ready: true },
  { to: "/providers", label: "Providers", icon: Pulse, group: "Observe", ready: true },
  { to: "/models", label: "Models", icon: StackSimple, group: "Observe", ready: true },
  { to: "/audit", label: "Audit Log", icon: ListChecks, group: "Observe", admin: true, ready: true },
  { to: "/projects", label: "Projects", icon: FolderSimple, group: "Manage", controlPlane: true, ready: true },
  { to: "/keys", label: "Virtual Keys", icon: Key, group: "Manage", controlPlane: true, ready: true },
  { to: "/policies", label: "Policies", icon: ShieldCheck, group: "Manage", controlPlane: true, ready: true },
  { to: "/budgets", label: "Budgets", icon: Wallet, group: "Manage", controlPlane: true, ready: true },
  { to: "/tools", label: "Tools", icon: Wrench, group: "Manage", controlPlane: true, ready: true },
  { to: "/mcp", label: "MCP Bindings", icon: PlugsConnected, group: "Manage", controlPlane: true, ready: true },
  { to: "/playground", label: "Playground", icon: PlayCircle, group: "Tools", ready: true },
];
