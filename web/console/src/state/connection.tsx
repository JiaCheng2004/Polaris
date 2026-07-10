import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { ApiError, request, type Conn } from "../api/http";

export type ConnStatus = "disconnected" | "connecting" | "connected";

export interface Capabilities {
  controlPlaneEnabled: boolean; // false when /v1/projects returns 404 control_plane_disabled
  isAdmin: boolean;
}

interface ConnectionCtx {
  conn: Conn | null;
  status: ConnStatus;
  caps: Capabilities;
  error: string | null;
  connect: (baseURL: string, token: string) => Promise<void>;
  disconnect: () => void;
}

const Ctx = createContext<ConnectionCtx | null>(null);

const LS_BASE = "polaris.baseURL";
const SS_TOKEN = "polaris.token"; // sessionStorage — cleared when the tab closes

function loadStored(): Conn | null {
  const baseURL = localStorage.getItem(LS_BASE);
  const token = sessionStorage.getItem(SS_TOKEN);
  if (baseURL && token) return { baseURL, token };
  return null;
}

export function ConnectionProvider({ children }: { children: ReactNode }) {
  const stored = loadStored();
  const [conn, setConn] = useState<Conn | null>(stored);
  const [status, setStatus] = useState<ConnStatus>(stored ? "connected" : "disconnected");
  const [caps, setCaps] = useState<Capabilities>({ controlPlaneEnabled: true, isAdmin: true });
  const [error, setError] = useState<string | null>(null);

  const connect = useCallback(async (rawBase: string, token: string) => {
    const baseURL = rawBase.trim().replace(/\/+$/, "");
    const candidate: Conn = { baseURL, token: token.trim() };
    setStatus("connecting");
    setError(null);

    // 1) Reachability (unauthenticated).
    try {
      await request(candidate, "/health");
    } catch (e) {
      setStatus("disconnected");
      setError(
        e instanceof ApiError
          ? `Gateway unreachable (${e.status}).`
          : "Gateway unreachable. Check the base URL, that Polaris is running, and CORS allows this origin."
      );
      throw e;
    }

    // 2) Admin + control-plane probe.
    let controlPlaneEnabled = true;
    let isAdmin = true;
    try {
      await request(candidate, "/v1/projects", { query: { include_archived: false } });
    } catch (e) {
      if (e instanceof ApiError) {
        if (e.status === 401) {
          setStatus("disconnected");
          setError("Authentication failed. The token is missing, invalid, revoked, or expired.");
          throw e;
        }
        if (e.status === 403) {
          setStatus("disconnected");
          setError("This key is not an admin key. Use the bootstrap admin key or an is_admin virtual key.");
          throw e;
        }
        if (e.status === 404 && e.code === "control_plane_disabled") {
          controlPlaneEnabled = false; // reachable + authed, but management routes are off
        } else {
          setStatus("disconnected");
          setError(e.message);
          throw e;
        }
      } else {
        setStatus("disconnected");
        setError("Connection probe failed.");
        throw e;
      }
    }

    localStorage.setItem(LS_BASE, baseURL);
    sessionStorage.setItem(SS_TOKEN, candidate.token);
    setConn(candidate);
    setCaps({ controlPlaneEnabled, isAdmin });
    setStatus("connected");
  }, []);

  const disconnect = useCallback(() => {
    sessionStorage.removeItem(SS_TOKEN);
    setConn(null);
    setStatus("disconnected");
    setError(null);
  }, []);

  const value = useMemo<ConnectionCtx>(
    () => ({ conn, status, caps, error, connect, disconnect }),
    [conn, status, caps, error, connect, disconnect]
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useConnection(): ConnectionCtx {
  const v = useContext(Ctx);
  if (!v) throw new Error("useConnection must be used within ConnectionProvider");
  return v;
}

/** Convenience for panels: the active connection, guaranteed non-null inside the app shell. */
export function useConn(): Conn {
  const { conn } = useConnection();
  if (!conn) throw new Error("No active connection");
  return conn;
}
