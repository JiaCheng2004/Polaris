import { useState } from "react";
import { useConnection } from "../state/connection";
import { useTheme } from "../state/theme";
import { ErrorNote } from "../components/ui";

export function Connection() {
  const { connect, status, error } = useConnection();
  const { theme, mode, toggleTheme, toggleMode } = useTheme();
  const [baseURL, setBaseURL] = useState(
    localStorage.getItem("polaris.baseURL") ?? "http://localhost:8080"
  );
  const [token, setToken] = useState("");
  const busy = status === "connecting";

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    connect(baseURL, token).catch(() => {});
  };

  return (
    <div style={{ minHeight: "100dvh", display: "grid", placeItems: "center", padding: 20, background: "var(--bg)" }}>
      <div style={{ position: "fixed", top: 16, right: 16, display: "flex", gap: 8 }}>
        <button className="btn btn-sm" onClick={toggleMode}>
          {mode === "advanced" ? "Advanced" : "Standard"}
        </button>
        <button className="btn btn-sm" onClick={toggleTheme}>
          {theme === "light" ? "Dark" : "Light"}
        </button>
      </div>

      <form
        onSubmit={submit}
        className="panel"
        style={{ width: "min(440px, 94vw)", padding: "28px" }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 9, marginBottom: 4 }}>
          <span aria-hidden style={{ width: 12, height: 12, borderRadius: "50%", background: "var(--accent)" }} />
          <h1 style={{ fontSize: 22 }}>Polaris Console</h1>
        </div>
        <p style={{ color: "var(--text-2)", fontSize: 13, marginTop: 0, marginBottom: 20 }}>
          Connect to a Polaris gateway with an admin credential.
        </p>

        <div className="field" style={{ marginBottom: 14 }}>
          <label htmlFor="base">Gateway base URL</label>
          <input id="base" className="input mono" value={baseURL} onChange={(e) => setBaseURL(e.target.value)} placeholder="http://localhost:8080" autoComplete="off" spellCheck={false} />
        </div>

        <div className="field" style={{ marginBottom: 8 }}>
          <label htmlFor="tok">Admin bearer token</label>
          <input id="tok" className="input mono" type="password" value={token} onChange={(e) => setToken(e.target.value)} placeholder="admin key or polaris-sk-live-…" autoComplete="off" spellCheck={false} />
        </div>

        <p className="meta" style={{ marginBottom: 16, lineHeight: 1.6, textTransform: "none" }}>
          This is the raw admin key whose hash is your gateway's
          <span className="mono"> bootstrap_admin_key_hash</span>, or an{" "}
          <span className="mono">is_admin</span> virtual key. First-time setup?{" "}
          <a href="https://github.com/JiaCheng2004/Polaris/blob/main/docs/CONSOLE.md#getting-an-admin-token" target="_blank" rel="noreferrer">
            How to get a token
          </a>
          .
        </p>

        {error && <div style={{ marginBottom: 14 }}><ErrorNote message={error} /></div>}

        <button className="btn btn-primary" type="submit" disabled={busy || !baseURL || !token} style={{ width: "100%", justifyContent: "center", height: 38 }}>
          {busy ? "Connecting…" : "Connect"}
        </button>

        <p className="meta" style={{ marginTop: 16, lineHeight: 1.6, textTransform: "none" }}>
          The token is a full-control admin credential. It is kept only in this tab's
          session storage and is cleared when the tab closes. Provider secrets are
          never stored or shown.
        </p>
      </form>
    </div>
  );
}
