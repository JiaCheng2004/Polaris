import { useEffect, type ReactNode } from "react";

function useEscape(onClose: () => void) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
}

function Backdrop({ onClose }: { onClose: () => void }) {
  return (
    <div
      onClick={onClose}
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,0.42)",
        zIndex: 40,
        animation: "fade var(--dur-1) var(--ease)",
      }}
    />
  );
}

/** Right-side drawer for create/detail forms. */
export function Drawer({
  title,
  onClose,
  children,
  footer,
}: {
  title: ReactNode;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
}) {
  useEscape(onClose);
  return (
    <>
      <Backdrop onClose={onClose} />
      <aside
        role="dialog"
        aria-modal="true"
        style={{
          position: "fixed",
          top: 0,
          right: 0,
          height: "100dvh",
          width: "min(460px, 94vw)",
          background: "var(--surface-1)",
          borderLeft: "1px solid var(--border-strong)",
          zIndex: 60,
          display: "flex",
          flexDirection: "column",
          boxShadow: "var(--shadow-pop)",
        }}
      >
        <header className="panel-head">
          <h2 className="panel-title">{title}</h2>
          <button className="btn btn-ghost btn-sm" onClick={onClose} aria-label="Close">
            ✕
          </button>
        </header>
        <div style={{ padding: "var(--pad-panel)", overflowY: "auto", flex: 1 }}>{children}</div>
        {footer && (
          <footer
            className="panel-head"
            style={{ borderTop: "1px solid var(--border)", borderBottom: "none", justifyContent: "flex-end" }}
          >
            {footer}
          </footer>
        )}
      </aside>
    </>
  );
}

/** Centered modal for confirmations and one-time reveals. */
export function Modal({
  title,
  onClose,
  children,
  footer,
  width = 460,
}: {
  title: ReactNode;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  width?: number;
}) {
  useEscape(onClose);
  return (
    <>
      <Backdrop onClose={onClose} />
      <div
        role="dialog"
        aria-modal="true"
        style={{
          position: "fixed",
          top: "50%",
          left: "50%",
          transform: "translate(-50%,-50%)",
          width: `min(${width}px, 94vw)`,
          maxHeight: "90dvh",
          background: "var(--surface-1)",
          border: "1px solid var(--border-strong)",
          borderRadius: "var(--radius-2)",
          zIndex: 60,
          display: "flex",
          flexDirection: "column",
          boxShadow: "var(--shadow-pop)",
        }}
      >
        <header className="panel-head">
          <h2 className="panel-title">{title}</h2>
          <button className="btn btn-ghost btn-sm" onClick={onClose} aria-label="Close">
            ✕
          </button>
        </header>
        <div style={{ padding: "var(--pad-panel)", overflowY: "auto" }}>{children}</div>
        {footer && (
          <footer
            className="panel-head"
            style={{ borderTop: "1px solid var(--border)", borderBottom: "none", justifyContent: "flex-end" }}
          >
            {footer}
          </footer>
        )}
      </div>
    </>
  );
}

export function ConfirmDialog({
  title,
  body,
  confirmLabel = "Confirm",
  danger,
  onConfirm,
  onClose,
}: {
  title: string;
  body: ReactNode;
  confirmLabel?: string;
  danger?: boolean;
  onConfirm: () => void;
  onClose: () => void;
}) {
  return (
    <Modal
      title={title}
      onClose={onClose}
      footer={
        <>
          <button className="btn btn-sm" onClick={onClose}>
            Cancel
          </button>
          <button
            className={`btn btn-sm ${danger ? "btn-danger" : "btn-primary"}`}
            onClick={() => {
              onConfirm();
              onClose();
            }}
          >
            {confirmLabel}
          </button>
        </>
      }
    >
      <div style={{ fontSize: 13, color: "var(--text-2)" }}>{body}</div>
    </Modal>
  );
}
