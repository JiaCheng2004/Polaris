import { useConnection } from "../state/connection";
import { useTheme } from "../state/theme";
import { KeyVal, PageHeader, Panel } from "../components/ui";

export function Settings() {
  const { conn, caps, disconnect } = useConnection();
  const { theme, mode, setTheme, setMode } = useTheme();

  return (
    <div>
      <PageHeader title="Settings" />
      <div className="grid">
        <Panel className="col-6" title="Connection">
          <KeyVal k="Gateway">{conn ? new URL(conn.baseURL).host : "not connected"}</KeyVal>
          <KeyVal k="Control plane">{caps.controlPlaneEnabled ? "enabled" : "disabled"}</KeyVal>
          <KeyVal k="Token">session-scoped, redacted</KeyVal>
          <div style={{ marginTop: 16 }}>
            <button className="btn btn-danger btn-sm" onClick={disconnect}>
              Disconnect
            </button>
          </div>
        </Panel>

        <Panel className="col-6" title="Appearance">
          <div className="field" style={{ marginBottom: 16 }}>
            <label>Theme</label>
            <div style={{ display: "flex", gap: 8 }}>
              {(["light", "dark"] as const).map((t) => (
                <button key={t} className={`btn btn-sm ${theme === t ? "btn-primary" : ""}`} onClick={() => setTheme(t)}>
                  {t}
                </button>
              ))}
            </div>
          </div>
          <div className="field">
            <label>Mode</label>
            <div style={{ display: "flex", gap: 8 }}>
              {(["standard", "advanced"] as const).map((m) => (
                <button key={m} className={`btn btn-sm ${mode === m ? "btn-primary" : ""}`} onClick={() => setMode(m)}>
                  {m}
                </button>
              ))}
            </div>
          </div>
        </Panel>
      </div>
    </div>
  );
}
