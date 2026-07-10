import { List, Moon, Sun } from "@phosphor-icons/react";
import { useConnection } from "../../state/connection";
import { useTheme } from "../../state/theme";

export function TopBar({ onMenu }: { onMenu: () => void }) {
  const { theme, mode, toggleTheme, toggleMode } = useTheme();
  const { conn, caps, disconnect } = useConnection();

  const host = conn ? new URL(conn.baseURL).host : "";

  return (
    <header
      style={{
        height: "var(--topbar-h)",
        display: "flex",
        alignItems: "center",
        gap: 12,
        padding: "0 var(--pad-panel)",
        borderBottom: "1px solid var(--border)",
        background: "var(--surface-1)",
        position: "sticky",
        top: 0,
        zIndex: 30,
      }}
    >
      <button className="btn btn-ghost btn-sm menu-btn" onClick={onMenu} aria-label="Open menu">
        <List size={18} />
      </button>

      <div className="meta" style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <span
          aria-hidden
          style={{
            width: 7,
            height: 7,
            borderRadius: "50%",
            background: "var(--status-good)",
          }}
        />
        {host}
        {!caps.controlPlaneEnabled && (
          <span className="tag" style={{ marginLeft: 6 }}>
            control plane off
          </span>
        )}
      </div>

      <div style={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: 8 }}>
        <button
          className="btn btn-sm"
          onClick={toggleMode}
          aria-pressed={mode === "advanced"}
          title="Toggle Advanced (tactical telemetry) mode"
        >
          {mode === "advanced" ? "Advanced" : "Standard"}
        </button>
        <button
          className="btn btn-ghost btn-sm"
          onClick={toggleTheme}
          aria-label={`Switch to ${theme === "light" ? "dark" : "light"} theme`}
        >
          {theme === "light" ? <Moon size={17} /> : <Sun size={17} />}
        </button>
        <button className="btn btn-sm" onClick={disconnect}>
          Disconnect
        </button>
      </div>
    </header>
  );
}
