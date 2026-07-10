import { NavLink } from "react-router-dom";
import { GearSix } from "@phosphor-icons/react";
import { useConnection } from "../../state/connection";
import { NAV, type NavItem } from "./nav";

const GROUPS: NavItem["group"][] = ["Observe", "Manage", "Tools"];

export function Sidebar({ onNavigate }: { onNavigate?: () => void }) {
  const { caps } = useConnection();

  const visible = NAV.filter((it) => {
    if (!it.ready) return false;
    if ((it.controlPlane || it.admin) && !caps.controlPlaneEnabled) return false;
    return true;
  });

  return (
    <nav
      aria-label="Console sections"
      style={{
        borderRight: "1px solid var(--border)",
        background: "var(--surface-1)",
        display: "flex",
        flexDirection: "column",
        height: "100dvh",
        position: "sticky",
        top: 0,
        overflowY: "auto",
      }}
    >
      <div
        style={{
          height: "var(--topbar-h)",
          display: "flex",
          alignItems: "center",
          gap: 8,
          padding: "0 16px",
          borderBottom: "1px solid var(--border)",
        }}
      >
        <span
          aria-hidden
          style={{ width: 10, height: 10, borderRadius: "50%", background: "var(--accent)" }}
        />
        <span style={{ fontWeight: 700, letterSpacing: "var(--letter-macro)", fontSize: 15 }}>
          Polaris
        </span>
      </div>

      <div style={{ padding: "10px 8px", flex: 1 }}>
        {GROUPS.map((g) => {
          const items = visible.filter((it) => it.group === g);
          if (items.length === 0) return null;
          return (
            <div key={g} style={{ marginBottom: 14 }}>
              <div className="meta" style={{ padding: "6px 12px" }}>
                {g}
              </div>
              {items.map((it) => (
                <NavLink
                  key={it.to}
                  to={it.to}
                  onClick={onNavigate}
                  style={({ isActive }) => ({
                    display: "flex",
                    alignItems: "center",
                    gap: 10,
                    padding: "8px 12px",
                    borderRadius: "var(--radius-1)",
                    color: isActive ? "var(--text-1)" : "var(--text-2)",
                    background: isActive ? "var(--accent-weak)" : "transparent",
                    fontSize: 13,
                    fontWeight: isActive ? 600 : 500,
                    textTransform: "var(--case-nav)" as never,
                    letterSpacing: "var(--letter-meta)",
                  })}
                >
                  <it.icon size={17} weight="regular" aria-hidden />
                  {it.label}
                </NavLink>
              ))}
            </div>
          );
        })}
      </div>

      <div style={{ padding: "8px", borderTop: "1px solid var(--border)" }}>
        <NavLink
          to="/settings"
          onClick={onNavigate}
          style={({ isActive }) => ({
            display: "flex",
            alignItems: "center",
            gap: 10,
            padding: "8px 12px",
            borderRadius: "var(--radius-1)",
            color: isActive ? "var(--text-1)" : "var(--text-2)",
            background: isActive ? "var(--accent-weak)" : "transparent",
            fontSize: 13,
            fontWeight: 500,
            textTransform: "var(--case-nav)" as never,
            letterSpacing: "var(--letter-meta)",
          })}
        >
          <GearSix size={17} aria-hidden />
          Settings
        </NavLink>
      </div>
    </nav>
  );
}
