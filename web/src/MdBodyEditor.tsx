import { useEffect, useRef } from 'react'
import { Select } from 'antd'
import { LanguageDescription } from '@codemirror/language'
import { EditorView } from '@codemirror/view'
import { CrepeBuilder } from '@milkdown/crepe/builder'
import { blockEdit } from '@milkdown/crepe/feature/block-edit'
import { codeMirror } from '@milkdown/crepe/feature/code-mirror'
import { cursor } from '@milkdown/crepe/feature/cursor'
import { imageBlock } from '@milkdown/crepe/feature/image-block'
import { linkTooltip } from '@milkdown/crepe/feature/link-tooltip'
import { listItem } from '@milkdown/crepe/feature/list-item'
import { placeholder } from '@milkdown/crepe/feature/placeholder'
import { table } from '@milkdown/crepe/feature/table'
import { toolbar } from '@milkdown/crepe/feature/toolbar'
import { topBar } from '@milkdown/crepe/feature/top-bar'
import type { ListenerManager } from '@milkdown/kit/plugin/listener'
import { insert, replaceAll } from '@milkdown/kit/utils'
import '@milkdown/crepe/theme/common/prosemirror.css'
import '@milkdown/crepe/theme/common/reset.css'
import '@milkdown/crepe/theme/common/block-edit.css'
import '@milkdown/crepe/theme/common/code-mirror.css'
import '@milkdown/crepe/theme/common/cursor.css'
import '@milkdown/crepe/theme/common/image-block.css'
import '@milkdown/crepe/theme/common/link-tooltip.css'
import '@milkdown/crepe/theme/common/list-item.css'
import '@milkdown/crepe/theme/common/placeholder.css'
import '@milkdown/crepe/theme/common/table.css'
import '@milkdown/crepe/theme/common/toolbar.css'
import '@milkdown/crepe/theme/common/top-bar.css'
import '@milkdown/crepe/theme/frame.css'

type MdBodyEditorProps = {
  adminDark: boolean
  editorKey: string
  imageSuggestions: string[]
  markdown: string
  onChange: (value: string) => void
  uploadImage: (file: File) => Promise<string>
}

const codeLanguages = [
  LanguageDescription.of({
    name: 'JavaScript',
    alias: ['js', 'jsx', 'typescript', 'ts', 'tsx'],
    extensions: ['js', 'jsx', 'ts', 'tsx', 'mjs', 'cjs'],
    load: async () => {
      const { javascript } = await import('@codemirror/lang-javascript')
      return javascript({ jsx: true, typescript: true })
    }
  }),
  LanguageDescription.of({
    name: 'Python',
    alias: ['py'],
    extensions: ['py', 'pyw'],
    load: async () => {
      const { python } = await import('@codemirror/lang-python')
      return python()
    }
  }),
  LanguageDescription.of({
    name: 'Go',
    alias: ['golang'],
    extensions: ['go'],
    load: async () => {
      const { go } = await import('@codemirror/lang-go')
      return go()
    }
  }),
  LanguageDescription.of({
    name: 'JSON',
    extensions: ['json', 'jsonc'],
    load: async () => {
      const { json } = await import('@codemirror/lang-json')
      return json()
    }
  }),
  LanguageDescription.of({
    name: 'YAML',
    alias: ['yml'],
    extensions: ['yaml', 'yml'],
    load: async () => {
      const { yaml } = await import('@codemirror/lang-yaml')
      return yaml()
    }
  }),
  LanguageDescription.of({
    name: 'HTML',
    extensions: ['html', 'htm'],
    load: async () => {
      const { html } = await import('@codemirror/lang-html')
      return html()
    }
  }),
  LanguageDescription.of({
    name: 'CSS',
    extensions: ['css'],
    load: async () => {
      const { css } = await import('@codemirror/lang-css')
      return css()
    }
  }),
  LanguageDescription.of({
    name: 'Markdown',
    alias: ['md'],
    extensions: ['md', 'markdown'],
    load: async () => {
      const { markdown } = await import('@codemirror/lang-markdown')
      return markdown()
    }
  }),
  LanguageDescription.of({
    name: 'SQL',
    extensions: ['sql'],
    load: async () => {
      const { sql } = await import('@codemirror/lang-sql')
      return sql()
    }
  })
]

const codeMirrorTheme = EditorView.theme({
  '&': {
    color: 'var(--admin-text)',
    backgroundColor: 'var(--admin-surface-2)'
  },
  '.cm-content': {
    caretColor: 'var(--admin-text)'
  },
  '.cm-cursor, .cm-dropCursor': {
    borderLeftColor: 'var(--admin-text)'
  },
  '&.cm-focused .cm-cursor': {
    borderLeftColor: 'var(--admin-text)'
  },
  '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection': {
    backgroundColor: 'rgba(var(--admin-primary-rgb), .28)'
  },
  '.cm-gutters': {
    color: 'var(--admin-muted)',
    backgroundColor: 'var(--admin-surface-2)',
    borderRight: '0'
  },
  '.cm-activeLine, .cm-activeLineGutter': {
    backgroundColor: 'rgba(var(--admin-primary-rgb), .08)'
  }
})

const topBarLabels = [
  'Bold',
  'Italic',
  'Strikethrough',
  'Inline code',
  'Bullet list',
  'Numbered list',
  'Task list',
  'Link',
  'Image',
  'Table',
  'Code block',
  'Quote',
  'Divider'
]

const toolbarLabels = [
  'Bold',
  'Italic',
  'Strikethrough',
  'Inline code',
  'Link'
]

function setControlLabel(element: Element, label: string) {
  element.setAttribute('title', label)
  if (!element.getAttribute('aria-label')) {
    element.setAttribute('aria-label', label)
  }
}

function labelMilkdownControls(root: HTMLElement) {
  const headingButton = root.querySelector('.milkdown-top-bar .top-bar-heading-button')
  if (headingButton) {
    setControlLabel(headingButton, 'Block style')
  }

  root.querySelectorAll('.milkdown-top-bar .top-bar-item').forEach((element, index) => {
    const label = topBarLabels[index]
    if (label) {
      setControlLabel(element, label)
    }
  })

  root.querySelectorAll('.milkdown-toolbar .toolbar-item').forEach((element, index) => {
    const label = toolbarLabels[index]
    if (label) {
      setControlLabel(element, label)
    }
  })

  root.querySelectorAll('.milkdown-code-block .language-button').forEach(element => {
    setControlLabel(element, 'Code language')
  })

  root.querySelectorAll('.milkdown-code-block .tools-button-group button').forEach(element => {
    const label = element.textContent?.trim() || 'Code block action'
    setControlLabel(element, label)
  })
}

export default function MdBodyEditor({ adminDark, editorKey, imageSuggestions, markdown, onChange, uploadImage }: MdBodyEditorProps) {
  const hostRef = useRef<HTMLDivElement | null>(null)
  const editorRef = useRef<CrepeBuilder | null>(null)
  const tooltipObserverRef = useRef<MutationObserver | null>(null)
  const onChangeRef = useRef(onChange)
  const uploadImageRef = useRef(uploadImage)
  const lastMarkdownRef = useRef(markdown)

  useEffect(() => {
    onChangeRef.current = onChange
  }, [onChange])

  useEffect(() => {
    uploadImageRef.current = uploadImage
  }, [uploadImage])

  useEffect(() => {
    if (!hostRef.current) return

    let cancelled = false
    const editor = new CrepeBuilder({
      root: hostRef.current,
      defaultValue: markdown
    })
      .addFeature(cursor)
      .addFeature(listItem)
      .addFeature(linkTooltip)
      .addFeature(codeMirror, {
        languages: codeLanguages,
        theme: codeMirrorTheme,
        searchPlaceholder: 'Search language',
        noResultText: 'No matching language'
      })
      .addFeature(imageBlock, {
        onUpload: (file: File) => uploadImageRef.current(file),
        inlineOnUpload: (file: File) => uploadImageRef.current(file),
        blockOnUpload: (file: File) => uploadImageRef.current(file)
      })
      .addFeature(blockEdit)
      .addFeature(topBar)
      .addFeature(toolbar)
      .addFeature(placeholder, {
        text: 'Start writing...',
        mode: 'block'
      })
      .addFeature(table)

    editor.on((listener: ListenerManager) => {
      listener.markdownUpdated((_ctx, nextMarkdown) => {
        lastMarkdownRef.current = nextMarkdown
        onChangeRef.current(nextMarkdown)
      })
    })

    editor.create()
      .then(() => {
        if (cancelled) {
          editor.destroy().catch(() => undefined)
          return
        }
        editorRef.current = editor
        if (hostRef.current) {
          labelMilkdownControls(hostRef.current)
          const observer = new MutationObserver(() => {
            if (hostRef.current) {
              labelMilkdownControls(hostRef.current)
            }
          })
          observer.observe(hostRef.current, { childList: true, subtree: true })
          tooltipObserverRef.current = observer
        }
      })
      .catch(() => undefined)

    return () => {
      cancelled = true
      tooltipObserverRef.current?.disconnect()
      tooltipObserverRef.current = null
      editorRef.current = null
      editor.destroy().catch(() => undefined)
    }
  }, [adminDark, editorKey])

  useEffect(() => {
    const editor = editorRef.current
    if (!editor) return
    if (lastMarkdownRef.current !== markdown) {
      lastMarkdownRef.current = markdown
      editor.editor.action(replaceAll(markdown, true))
    }
  }, [markdown])

  function insertImage(url: string) {
    const editor = editorRef.current
    if (!editor) return
    editor.editor.action(insert(`\n\n![image](${url})\n`))
  }

  return <div className={adminDark ? 'milkdownMarkdownEditor milkdownDark' : 'milkdownMarkdownEditor'}>
    {imageSuggestions.length > 0 && <div className="milkdownAssetBar">
      <Select
        size="small"
        className="milkdownAssetSelect"
        placeholder="Insert uploaded image"
        value={undefined}
        options={imageSuggestions.map(url => ({ label: url, value: url }))}
        onChange={insertImage}
      />
    </div>}
    <div ref={hostRef} />
  </div>
}
