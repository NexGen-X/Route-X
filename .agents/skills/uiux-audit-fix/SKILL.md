---
name: uiux-audit-fix
description: >
  Use when the user asks for a UI/UX audit, design review, accessibility
  review, or "why does this screen feel off" style requests on a web or
  mobile interface (from code, screenshots, or a live URL). Produces a
  structured findings report with severity, root cause, and a concrete
  fix (minimal + ideal) for each issue — not just aesthetic opinions.
  Do not use for pure backend/API review, performance profiling unrelated
  to perceived UX, or copywriting-only tasks with no interface involved.
---

# UI/UX Audit & Fix

## Goal

Turn a vague "review the UI/UX" request into a reproducible audit that:
1. finds concrete, evidence-backed issues (not vibes),
2. explains the root cause of each issue, and
3. ships a fix the user can apply immediately, plus a way to verify it worked.

> Note on methodology: there is no published documentation describing a
> distinct "thinking style" for Claude Fable 5.1 to model this on, so
> this skill instead codifies a rigorous, field-tested audit method
> (heuristic evaluation + root-cause analysis) rather than imitating a
> specific model's reasoning.

## Requirements

Before starting, make sure at least one of these is available:
- Source code of the relevant component(s) (React/Vue/HTML/CSS/etc.), and/or
- Screenshots at 3 widths (mobile ~375px, tablet ~768px, desktop ~1440px), and/or
- A live URL that can be opened/inspected.

If none of these are available, ask the user for at least one before proceeding —
do not guess at a UI you cannot see.

### Stack-specific defaults (Route-X)
When the audited code lives in a React + Tailwind frontend (Vite, TanStack
Query, Recharts, Lucide React — e.g. the Route-X `web/` app), prefer fixes
written in that stack's idioms instead of generic CSS/JS:
- **State styling**: use Tailwind state variants (`hover:`, `focus-visible:`,
  `active:`, `disabled:`) instead of hand-written CSS pseudo-classes.
  Example: `className="focus-visible:ring-2 focus-visible:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed"`.
- **Loading/error/empty states**: derive them from TanStack Query's
  `isLoading` / `isError` / `data` flags, not a generic spinner bolted on
  separately. Example pattern:
  ```tsx
  const { data, isLoading, isError } = useQuery(...);
  if (isLoading) return <Skeleton />;
  if (isError) return <ErrorState retry={refetch} />;
  if (!data?.length) return <EmptyState />;
  ```
- **Charts (Recharts)**: dashboards/metrics must not rely on color alone —
  add direct labels or a legend, provide a `title`/`aria-label` on the
  chart container, and give tooltips sufficient contrast in both themes.
- **Icons (Lucide React)**: purely decorative icons get `aria-hidden="true"`;
  icon-only buttons need an accessible name via `aria-label` (Lucide icons
  carry no text alternative on their own).
- **Class merging**: when proposing conditional classes, use the project's
  existing `clsx` / `tailwind-merge` utilities rather than string
  concatenation, to avoid Tailwind class-conflict bugs.

This section is a default, not a constraint — if the audited code is not
Route-X's React/Tailwind stack, fall back to the generic examples in Step 3
below.

## Steps

### 1. Reasoning framework (apply per issue, never skip a step)
```
OBSERVE   -> what is literally visible/measurable?
DIAGNOSE  -> which UX heuristic / WCAG criterion is violated?
ROOT CAUSE-> why does this happen technically or in the design system?
IMPACT    -> who is affected, how often, how severe?
FIX       -> minimal viable fix AND ideal fix, with concrete code/example
VERIFY    -> how to confirm the fix actually worked
```
Never jump straight from OBSERVE to FIX — a misdiagnosed root cause produces
a patch on the symptom (e.g. bumping one button's contrast instead of fixing
the whole color-token set that fails WCAG).

### 2. Run the checklist (all 9 dimensions, no sampling)
1. **Visual hierarchy & typography** — does size/weight reflect priority? consistent type scale? readable line-length (45–75 chars)?
2. **Color & contrast (WCAG 2.2)** — 4.5:1 normal text / 3:1 large text; color never the only status signal; check light AND dark mode separately.
3. **Spacing & layout consistency** — spacing on a token scale (4/8/12/16/24…) vs arbitrary values; identical components share identical internal spacing.
4. **Interaction & feedback states** — default/hover/focus/active/disabled/loading/error all defined; visible focus ring for keyboard nav; async actions show feedback (skeleton/spinner), never a silently frozen UI.
5. **Accessibility** — full keyboard operability (Tab/Enter/Esc); 44x44px min touch target; ARIA only where native semantics fall short; meaningful `alt` text.
6. **Responsiveness** — test at 375 / 768 / 1440px minimum; no overflow, no stacked/overlapping controls on resize.
7. **Information architecture & navigation** — is current location always clear (active nav state, breadcrumb)? how many taps/clicks for the happy path, and can it be shortened?
8. **Microcopy & error messaging** — errors state what went wrong AND what to do next (never "Error 500"); button labels use clear verbs.
9. **Perceived performance** — skeleton/placeholder while loading; transitions ≤300ms for routine actions.

### 3. Write findings using this exact structure per issue
```markdown
### [SEVERITY] Short title
- **Location**: file/component/URL + selector or screenshot ref
- **Observation**: factual description of what is seen
- **Violated principle**: named heuristic/guideline (e.g. Nielsen Heuristic #1, WCAG 1.4.3, Fitts's Law)
- **Root cause**: the real technical/design cause
- **Impact**: who/how often/how severe
- **Fix — Minimal**: fastest mitigation
- **Fix — Ideal**: full fix, with code/CSS example where relevant
- **Verification**: tool or manual test step to confirm it's fixed
```

Severity scale:
| Level | Criteria |
|---|---|
| Critical | Blocks a core task, or a basic accessibility failure that locks some users out entirely |
| High | Breaks the main flow, causes significant confusion |
| Medium | Annoying but has a workaround; visible inconsistency |
| Low | Polish / nice-to-have, no task impact |

### 4. Close the report
- Order findings Critical → Low.
- End with a summary: count per severity + top 3 priority recommendations.
- If asked to implement fixes: apply "Minimal" fixes for Critical/High first, then confirm with the user before doing "Ideal" fixes that touch structure broadly.

## Verification

The skill's own output is correct when every listed finding has all 6
reasoning-framework fields filled in, no two issues are merged into one
bullet, and no fix is proposed without a verification step. Reject and
rewrite any finding that only says something is "not modern enough" or
"looks off" without a diagnosed root cause and a testable fix.

## Anti-patterns (reject these outright)
- "This looks outdated" with no root cause or concrete fix.
- A fix with no verification step.
- Multiple issues merged into a single bullet.
- Cosmetic patches that mask a structural root cause (e.g. adding `!important` instead of fixing CSS specificity).
