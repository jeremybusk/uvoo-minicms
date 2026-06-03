# Navigation Menus

Uvoo-MiniCMS supports nested public navigation items. Menu items are managed from the admin `Site` tab.

## Item Types

Use `Link` for a real destination:

- Internal pages such as `/about`
- External URLs such as `https://example.com`
- Parent overview pages that also have child menu items

Use `Section` for a non-clickable parent or label:

- Grouping children under headings such as `Services`, `Resources`, or `Newsletters`
- Mobile and drawer menus where the parent row should expand and collapse children
- Header dropdowns where the parent is not its own page

Section items do not use a URL and are rendered as buttons when they have children. Link items remain normal links; if they also have children, a separate chevron button expands or collapses the submenu.

## Mobile Behavior

On small screens, parent items with children collapse by default and show a chevron control. The chevron uses `aria-expanded` so screen readers can announce whether the submenu is open.

## Recommended Structure

Prefer a real parent `Link` when the parent page is useful as an overview page. Prefer a `Section` when the parent is only a category heading.

Example:

```text
Home                  Link     /
Services              Link     /services
  Installation        Link     /services/installation
  Support             Link     /services/support
Resources             Section
  Blog                Link     /blog
  Videos              Link     /videos
```

