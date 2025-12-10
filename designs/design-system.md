# AG-Godownload Design System

## Design Philosophy

**Inspiration Sources:**
- **Google Drive**: Clean, minimal, professional with subtle shadows and clear hierarchy
- **StashApp**: Dark-first, media-focused, efficient use of space
- **MetArt**: Premium, elegant, photography-focused with emphasis on content

**Core Principles:**
1. **Content First** - UI should never compete with media
2. **Minimal & Clean** - Remove visual noise, use whitespace effectively
3. **Professional** - Sophisticated, not playful
4. **Efficient** - Dense information display without clutter
5. **Dark-Optimized** - Designed for dark mode first, light mode secondary

---

## Color Palette

### Dark Mode (Primary)
```css
--bg-primary: #0f0f0f;        /* Deep black background */
--bg-secondary: #1a1a1a;      /* Card backgrounds */
--bg-tertiary: #242424;       /* Hover states */
--bg-elevated: #2a2a2a;       /* Modals, dropdowns */

--text-primary: #e8e8e8;      /* Main text */
--text-secondary: #a0a0a0;    /* Secondary text */
--text-tertiary: #707070;     /* Disabled, hints */

--border-subtle: #2a2a2a;     /* Barely visible borders */
--border-primary: #3a3a3a;    /* Standard borders */
--border-focus: #4a4a4a;      /* Focus states */

--accent-primary: #3b82f6;    /* Blue - primary actions */
--accent-hover: #2563eb;      /* Darker blue */
--accent-danger: #ef4444;     /* Red - destructive */
--accent-success: #10b981;    /* Green - success */
```

### Light Mode (Secondary)
```css
--bg-primary: #ffffff;
--bg-secondary: #f8f9fa;
--bg-tertiary: #f1f3f5;
--bg-elevated: #ffffff;

--text-primary: #1a1a1a;
--text-secondary: #6b7280;
--text-tertiary: #9ca3af;

--border-subtle: #f1f3f5;
--border-primary: #e5e7eb;
--border-focus: #d1d5db;
```

---

## Typography

### Font Stack
```css
font-family: -apple-system, BlinkMacSystemFont, 'Inter', 'Segoe UI', 'Roboto', sans-serif;
```

### Scale
- **Display**: 2rem (32px) - Page titles
- **Heading 1**: 1.5rem (24px) - Section headers
- **Heading 2**: 1.25rem (20px) - Subsections
- **Body**: 0.875rem (14px) - Default text
- **Small**: 0.75rem (12px) - Metadata, labels
- **Tiny**: 0.6875rem (11px) - Timestamps, counts

### Weights
- **Regular**: 400 - Body text
- **Medium**: 500 - Emphasis
- **Semibold**: 600 - Headings, buttons

---

## Spacing System

```css
--space-1: 4px;
--space-2: 8px;
--space-3: 12px;
--space-4: 16px;
--space-5: 20px;
--space-6: 24px;
--space-8: 32px;
--space-10: 40px;
--space-12: 48px;
```

---

## Component Patterns

### Cards
- **Background**: `--bg-secondary`
- **Border**: `1px solid --border-subtle` (barely visible)
- **Border Radius**: `8px` (subtle, not rounded)
- **Padding**: `16px` (compact)
- **Shadow**: None on default, subtle on hover
- **Hover**: Slight border color change, no transform

### Buttons

#### Primary
```css
background: --accent-primary;
color: white;
border: none;
padding: 8px 16px;
border-radius: 6px;
font-size: 14px;
font-weight: 500;
```

#### Secondary
```css
background: transparent;
color: --text-primary;
border: 1px solid --border-primary;
```

#### Ghost
```css
background: transparent;
color: --text-secondary;
border: none;
hover: background: --bg-tertiary;
```

### Icons
- **Size**: 20px (standard), 16px (small), 24px (large)
- **Style**: Outline/stroke, not filled
- **Color**: `--text-secondary` default, `--text-primary` on hover

### Sidebar (Admin Panel)
- **Width**: 320px (narrower than current)
- **Background**: `--bg-elevated`
- **Border**: `1px solid --border-primary` (left side)
- **Shadow**: Subtle, not dramatic
- **Animation**: Fast (150ms), smooth

### FAB (Floating Action Button)
- **Size**: 48px (smaller than current 56px)
- **Background**: `--bg-elevated` with border
- **Icon**: Outline style, not emoji
- **Shadow**: Subtle elevation
- **Position**: 24px from bottom-right

---

## Layout Principles

### Grid System
- **Gallery Grid**: `minmax(200px, 1fr)` - Smaller, denser
- **Gap**: `12px` - Tighter spacing
- **Max Width**: `1400px` - Wider for media

### Density
- **Compact Mode**: Default - maximize content
- **Comfortable Mode**: Optional - more breathing room

### Navigation
- **Header Height**: `56px` (slim)
- **Tabs**: Underline style, not pills
- **Active State**: Border-bottom, not background

---

## Imagery

### Thumbnails
- **Aspect Ratio**: Maintain original (no forced 16:9)
- **Fit**: `cover` for thumbnails, `contain` for lightbox
- **Loading**: Skeleton with subtle shimmer
- **Placeholder**: Gradient from `--bg-secondary` to `--bg-tertiary`

### Overlays
- **Play Icons**: Subtle, white with low opacity
- **Badges**: Small, corner-positioned, minimal
- **Metadata**: Only on hover, dark overlay with white text

---

## Interaction Design

### Hover States
- **Cards**: Border color change only
- **Buttons**: Slight background darkening
- **Links**: Underline appears
- **No**: Scale transforms, dramatic shadows

### Focus States
- **Outline**: `2px solid --accent-primary` with `2px offset`
- **Ring**: Subtle, not glowing

### Transitions
- **Fast**: `150ms` - UI interactions
- **Base**: `200ms` - Standard
- **Slow**: `300ms` - Modals, sidebars
- **Easing**: `cubic-bezier(0.4, 0, 0.2, 1)` - Material Design

---

## Accessibility

### Contrast Ratios
- **Text**: Minimum 4.5:1
- **Large Text**: Minimum 3:1
- **Interactive**: Minimum 3:1

### Focus Indicators
- Always visible
- High contrast
- 2px minimum width

---

## Implementation Notes

### Remove
- ❌ Emoji icons (use SVG icons instead)
- ❌ Rounded pill buttons
- ❌ Heavy shadows
- ❌ Transform animations on hover
- ❌ Bright, saturated colors
- ❌ Gradient backgrounds

### Add
- ✅ Subtle borders
- ✅ Minimal shadows
- ✅ Icon-only buttons with tooltips
- ✅ Monochrome color scheme with accent
- ✅ Consistent 8px grid
- ✅ Professional typography

---

## Reference Screenshots

### Google Drive
- Clean white/gray backgrounds
- Minimal borders
- Icon-based actions
- Dense grid layout
- Subtle hover states

### StashApp
- Dark-first design
- Media-focused layout
- Efficient use of space
- Metadata on hover
- Professional typography

### MetArt
- Content-first approach
- Elegant spacing
- Premium feel
- Photography emphasis
- Minimal UI chrome
