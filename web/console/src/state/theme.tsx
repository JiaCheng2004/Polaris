import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

export type Theme = "light" | "dark";
export type Mode = "standard" | "advanced";

interface ThemeCtx {
  theme: Theme;
  mode: Mode;
  toggleTheme: () => void;
  toggleMode: () => void;
  setTheme: (t: Theme) => void;
  setMode: (m: Mode) => void;
}

const Ctx = createContext<ThemeCtx | null>(null);

function initialTheme(): Theme {
  const saved = localStorage.getItem("polaris.theme");
  if (saved === "light" || saved === "dark") return saved;
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}
function initialMode(): Mode {
  return localStorage.getItem("polaris.mode") === "advanced" ? "advanced" : "standard";
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<Theme>(initialTheme);
  const [mode, setMode] = useState<Mode>(initialMode);

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
    localStorage.setItem("polaris.theme", theme);
  }, [theme]);
  useEffect(() => {
    document.documentElement.setAttribute("data-mode", mode);
    localStorage.setItem("polaris.mode", mode);
  }, [mode]);

  const value = useMemo<ThemeCtx>(
    () => ({
      theme,
      mode,
      toggleTheme: () => setTheme((t) => (t === "light" ? "dark" : "light")),
      toggleMode: () => setMode((m) => (m === "standard" ? "advanced" : "standard")),
      setTheme,
      setMode,
    }),
    [theme, mode]
  );

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useTheme(): ThemeCtx {
  const v = useContext(Ctx);
  if (!v) throw new Error("useTheme must be used within ThemeProvider");
  return v;
}
