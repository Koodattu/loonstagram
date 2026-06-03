# Product

## Register

product

## Users

InstaFix serves casual Discord users who want a fixed Instagram share URL without understanding URL rewriting, power users who know the canonical paths and edit links directly, and preview crawlers such as Discordbot that need immediate metadata HTML.

## Product Purpose

The product turns supported public Instagram post, reel, and video URLs into shareable InstaFix URLs. Human visitors should land on the original Instagram page, while crawlers receive escaped Open Graph and Twitter Card metadata that produces a useful Discord preview.

## Brand Personality

Practical, quiet, reliable. The interface should feel like a small utility that does one job quickly, with clear validation and no marketing framing.

## Anti-references

Avoid landing-page hero treatments, decorative dashboards, full frontend frameworks, account flows, admin surfaces, and any UI that implies media archival or private Instagram access. Avoid visually loud Discord or Instagram mimicry.

## Design Principles

- Make conversion the first visible action.
- Keep human redirect behavior predictable.
- Treat metadata scraping as best effort, with graceful fallback previews.
- Keep unsupported or risky inputs out of the system.
- Use restrained product UI patterns that stay readable and fast.

## Accessibility & Inclusion

Target WCAG 2.1 AA for the small web UI. Preserve keyboard access, visible focus states, readable contrast, plain error text, reduced-motion friendly transitions, and semantic form markup.
