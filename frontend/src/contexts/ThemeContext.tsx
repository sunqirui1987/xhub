import React, { createContext, useContext, useState, useEffect, ReactNode } from "react";
import { t } from "@/i18n";

interface ThemeContextType {
  logoUrl: string | null;
  setLogoUrl: (url: string | null) => void;
  logoUrlDark: string | null;
  setLogoUrlDark: (url: string | null) => void;
  faviconUrl: string | null;
  setFaviconUrl: (url: string | null) => void;
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

export const useTheme = () => {
  const context = useContext(ThemeContext);
  if (!context) {
    throw new Error(t("useTheme must be used within a ThemeProvider"));
  }
  return context;
};

interface ThemeProviderProps {
  children: ReactNode;
  accessToken?: string | null;
}

export const ThemeProvider: React.FC<ThemeProviderProps> = ({ children }) => {
  const [logoUrl, setLogoUrl] = useState<string | null>(null);
  const [logoUrlDark, setLogoUrlDark] = useState<string | null>(null);
  const [faviconUrl, setFaviconUrl] = useState<string | null>(null);

  useEffect(() => {
    if (faviconUrl) {
      const existingLinks = document.querySelectorAll("link[rel*='icon']");
      if (existingLinks.length > 0) {
        existingLinks.forEach((link) => {
          (link as HTMLLinkElement).href = faviconUrl;
        });
      } else {
        const link = document.createElement("link");
        link.rel = "icon";
        link.href = faviconUrl;
        document.head.appendChild(link);
      }
    }
  }, [faviconUrl]);

  return (
    <ThemeContext.Provider value={{ logoUrl, setLogoUrl, logoUrlDark, setLogoUrlDark, faviconUrl, setFaviconUrl }}>
      {children}
    </ThemeContext.Provider>
  );
};
