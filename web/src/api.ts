export type Page = {
  id:number
  slug:string
  path:string
  title:string
  meta_description:string
  content_type:'page'|'post'
  markdown?:string
  tags:string
  published:boolean
  created_at:string
  updated_at:string
}
export type NavItem = { id:string; parent_id:string; label:string; url:string; external:boolean; enabled:boolean }
export type Asset = { id:number; name:string; url:string; size:number; created_at:string }
export type ACLRule = { id?:number; scope:'all'|'admin'|'public'; action:'allow'|'deny'; cidr:string; note:string; enabled:boolean }
export type ACLSettings = {
  admin_default:'allow'|'deny'
  public_default:'allow'|'deny'
  admin_allow_countries:string
  admin_deny_countries:string
  public_allow_countries:string
  public_deny_countries:string
  rules:ACLRule[]
}
export type SiteSettings = {
  site_name:string
  logo_url:string
  favicon_url:string
  default_theme:'light'|'dark'
  public_primary_color:string
  public_secondary_color:string
  public_header_style:'neutral'|'accent-line'|'accent-bg'
  admin_theme:'light'|'dark'
  admin_primary_color:string
  admin_secondary_color:string
  admin_palette:'slate'|'forest'|'ember'|'mono'|'custom'
  footer_markdown:string
  menu:NavItem[]
  logo_enabled:boolean
  favicon_enabled:boolean
  menu_enabled:boolean
  footer_enabled:boolean
  theme_toggle_enabled:boolean
  icons_enabled:boolean
  search_enabled:boolean
  nav_layout:'top'|'side'
}
export type ImportPage = {
  slug:string
  path:string
  title:string
  meta_description:string
  content_type:'page'|'post'
  tags:string
  markdown:string
  source_url:string
  published:boolean
  exists:boolean
}
export type ImportOptions = {
  url:string
  max_pages:number
  include_posts:boolean
  import_menu:boolean
  publish:boolean
  update_existing:boolean
  download_images:boolean
}
export type ImportResult = {
  source:string
  base_url:string
  pages:ImportPage[]
  menu:NavItem[]
  imported:number
  skipped:number
  errors:string[]
  existing:number
  wordpress:boolean
  sitemap_url:string
  preview_limit:number
}

const baseURL = new URL('/cms.v1.CMSService/', window.location.href)
baseURL.username = ''
baseURL.password = ''
async function rpc<T>(name: string, body: Record<string, unknown> = {}): Promise<T> {
  const r = await fetch(new URL(name, baseURL).toString(), { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
  if (!r.ok) throw new Error(await r.text())
  return r.json()
}
export const api = {
  listPages: () => rpc<{pages:Page[]}>('ListPages'),
  getPage: (slug:string) => rpc<{page:Page}>('GetPage', { slug }),
  savePage: (page: Partial<Page>) => rpc<{page:Page}>('SavePage', page),
  deletePage: (slug:string) => rpc<{ok:boolean}>('DeletePage', { slug }),
  getSettings: () => rpc<{settings:SiteSettings}>('GetSettings'),
  saveSettings: (settings: SiteSettings) => rpc<{settings:SiteSettings}>('SaveSettings', settings),
  listAssets: () => rpc<{assets:Asset[]}>('ListAssets'),
  uploadFile: (name:string, data:string) => rpc<{asset:Asset}>('UploadFile', { name, data }),
  setSiteImage: (kind:'logo'|'favicon', name:string, data:string, url = '') => rpc<{asset:Asset, settings:SiteSettings}>('SetSiteImage', { kind, name, data, url }),
  getACL: () => rpc<{acl:ACLSettings}>('GetACL'),
  saveACL: (acl: ACLSettings) => rpc<{acl:ACLSettings}>('SaveACL', acl),
  importPreview: (opts: ImportOptions) => rpc<{import:ImportResult}>('ImportPreview', opts),
  importSite: (opts: ImportOptions) => rpc<{import:ImportResult}>('ImportSite', opts)
}
