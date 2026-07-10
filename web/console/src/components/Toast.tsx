import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";

type ToastKind = "info" | "success" | "error";
interface Toast {
  id: number;
  kind: ToastKind;
  message: string;
}
interface ToastCtx {
  push: (message: string, kind?: ToastKind) => void;
}
const Ctx = createContext<ToastCtx | null>(null);
let seq = 1;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const push = useCallback((message: string, kind: ToastKind = "info") => {
    const id = seq++;
    setToasts((t) => [...t, { id, kind, message }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 4200);
  }, []);
  const value = useMemo(() => ({ push }), [push]);
  return (
    <Ctx.Provider value={value}>
      {children}
      <div
        style={{
          position: "fixed",
          right: 16,
          bottom: 16,
          display: "flex",
          flexDirection: "column",
          gap: 8,
          zIndex: 80,
        }}
      >
        {toasts.map((t) => (
          <div
            key={t.id}
            role="status"
            className="num"
            style={{
              padding: "10px 14px",
              borderRadius: "var(--radius-1)",
              border: "1px solid var(--border-strong)",
              background: "var(--surface-1)",
              color:
                t.kind === "error"
                  ? "var(--danger)"
                  : t.kind === "success"
                    ? "var(--status-good)"
                    : "var(--text-1)",
              boxShadow: "var(--shadow-pop)",
              fontSize: 13,
              maxWidth: 360,
            }}
          >
            {t.message}
          </div>
        ))}
      </div>
    </Ctx.Provider>
  );
}

export function useToast(): ToastCtx {
  const v = useContext(Ctx);
  if (!v) throw new Error("useToast must be used within ToastProvider");
  return v;
}
