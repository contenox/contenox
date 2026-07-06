import { defineConfig } from "astro/config";
import tailwindcss from "@tailwindcss/vite";
import sitemap from "@astrojs/sitemap";

export default defineConfig({
  site: "https://contenox.com",
  output: "static",
  integrations: [sitemap()],
  markdown: {
    remarkPlugins: [(await import("./src/lib/remark-md-links.mjs")).default],
    shikiConfig: {
      themes: { light: "github-light", dark: "github-dark" },
      defaultColor: false,
    },
  },
  vite: {
    plugins: [tailwindcss()],
  },
  // contenox.com was heavily indexed as a Next.js app. Every route the old
  // site served (or redirected) must keep resolving on the static site: known
  // routes get explicit redirect pages here; everything else (tokened invites,
  // /bob/*, /api/*) is caught by the 404 page, which forwards to the homepage.
  redirects: {
    // Old marketing/docs aliases the Next config carried.
    "/features": "/docs/guide/quickstart/",
    "/docs/beam": "/docs/guide/quickstart/",
    "/docs/guide/beam": "/docs/guide/quickstart/",
    "/docs/guide/introduction": "/docs/guide/quickstart/",
    // Retired EE/commerce surfaces.
    "/pricing": "/",
    "/services": "/",
    "/cloud": "/",
    "/login": "/",
    "/signup": "/",
    "/forgot-password": "/",
    "/reset-password": "/",
    "/invite": "/",
    "/admin": "/",
    "/admin/login": "/",
    "/admin/bob": "/",
    "/bob": "/",
    "/pilot": "/",
    "/pilot/managed-mcp": "/",
    "/pilot/success": "/",
    "/pilot/cancel": "/",
    // Pages that return with the content pass; keep the URLs alive meanwhile.
    "/about": "/",
    "/legal": "/",
  },
});
