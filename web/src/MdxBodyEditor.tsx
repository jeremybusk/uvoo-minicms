import { useEffect, useMemo, useRef } from 'react'
import { Button, Select, Space } from 'antd'
import { EditorState, Extension } from '@codemirror/state'
import { EditorView, ViewUpdate, drawSelection, highlightActiveLine, highlightActiveLineGutter, keymap, lineNumbers } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, indentWithTab, redo, undo } from '@codemirror/commands'
import { bracketMatching, defaultHighlightStyle, indentOnInput, syntaxHighlighting } from '@codemirror/language'
import { markdown } from '@codemirror/lang-markdown'
import { highlightSelectionMatches, searchKeymap } from '@codemirror/search'

type MdxBodyEditorProps = {
  adminDark: boolean
  editorKey: string
  imageSuggestions: string[]
  markdown: string
  onChange: (value: string) => void
  uploadImage: (file: File) => Promise<string>
}

export default function MdxBodyEditor({ adminDark, editorKey, imageSuggestions, markdown: markdownText, onChange, uploadImage }: MdxBodyEditorProps) {
  const hostRef = useRef<HTMLDivElement | null>(null)
  const viewRef = useRef<EditorView | null>(null)
  const fileRef = useRef<HTMLInputElement | null>(null)
  const onChangeRef = useRef(onChange)

  useEffect(() => {
    onChangeRef.current = onChange
  }, [onChange])

  const extensions = useMemo<Extension[]>(() => [
    lineNumbers(),
    highlightActiveLineGutter(),
    history(),
    drawSelection(),
    indentOnInput(),
    bracketMatching(),
    markdown(),
    syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
    highlightSelectionMatches(),
    keymap.of([
      indentWithTab,
      ...defaultKeymap,
      ...historyKeymap,
      ...searchKeymap
    ]),
    EditorView.lineWrapping,
    EditorView.theme({
      '&': {
        minHeight: '540px'
      },
      '.cm-scroller': {
        minHeight: '540px'
      }
    }, { dark: adminDark }),
    EditorView.updateListener.of((update: ViewUpdate) => {
      if (update.docChanged) {
        onChangeRef.current(update.state.doc.toString())
      }
    })
  ], [adminDark])

  useEffect(() => {
    if (!hostRef.current) return
    const state = EditorState.create({ doc: markdownText, extensions })
    const view = new EditorView({ parent: hostRef.current, state })
    viewRef.current = view
    return () => {
      view.destroy()
      viewRef.current = null
    }
  }, [editorKey, extensions])

  useEffect(() => {
    const view = viewRef.current
    if (!view) return
    const current = view.state.doc.toString()
    if (current !== markdownText) {
      view.dispatch({
        changes: { from: 0, to: current.length, insert: markdownText }
      })
    }
  }, [markdownText])

  function focusEditor() {
    viewRef.current?.focus()
  }

  function replaceSelection(before: string, after = before, fallback = 'text') {
    const view = viewRef.current
    if (!view) return
    const selection = view.state.selection.main
    const selected = view.state.sliceDoc(selection.from, selection.to) || fallback
    const insert = `${before}${selected}${after}`
    view.dispatch({
      changes: { from: selection.from, to: selection.to, insert },
      selection: { anchor: selection.from + before.length, head: selection.from + before.length + selected.length },
      scrollIntoView: true
    })
    focusEditor()
  }

  function prefixLines(prefix: string) {
    const view = viewRef.current
    if (!view) return
    const selection = view.state.selection.main
    const fromLine = view.state.doc.lineAt(selection.from)
    const toLine = view.state.doc.lineAt(selection.to)
    const changes = []
    for (let lineNo = fromLine.number; lineNo <= toLine.number; lineNo += 1) {
      changes.push({ from: view.state.doc.line(lineNo).from, insert: prefix })
    }
    view.dispatch({ changes, scrollIntoView: true })
    focusEditor()
  }

  function insertText(text: string, cursorOffset = text.length) {
    const view = viewRef.current
    if (!view) return
    const selection = view.state.selection.main
    view.dispatch({
      changes: { from: selection.from, to: selection.to, insert: text },
      selection: { anchor: selection.from + cursorOffset },
      scrollIntoView: true
    })
    focusEditor()
  }

  function insertLink() {
    const url = window.prompt('URL')
    if (!url) return
    const view = viewRef.current
    const selection = view?.state.selection.main
    const label = view && selection ? view.state.sliceDoc(selection.from, selection.to) || 'link' : 'link'
    replaceSelection('[', `](${url})`, label)
  }

  function insertImage(url: string) {
    insertText(`![image](${url})`)
  }

  async function uploadAndInsert(file: File) {
    const url = await uploadImage(file)
    insertImage(url)
  }

  return <div className={adminDark ? 'markdownEditor dark-theme' : 'markdownEditor'}>
    <div className="markdownToolbar">
      <Space wrap size={6}>
        <Button size="small" onClick={() => viewRef.current && undo(viewRef.current)}>Undo</Button>
        <Button size="small" onClick={() => viewRef.current && redo(viewRef.current)}>Redo</Button>
        <Button size="small" onClick={() => replaceSelection('**', '**', 'bold')}>B</Button>
        <Button size="small" onClick={() => replaceSelection('_', '_', 'italic')}>I</Button>
        <Button size="small" onClick={() => replaceSelection('`', '`', 'code')}>Code</Button>
        <Button size="small" onClick={() => prefixLines('# ')}>H1</Button>
        <Button size="small" onClick={() => prefixLines('## ')}>H2</Button>
        <Button size="small" onClick={() => prefixLines('- ')}>List</Button>
        <Button size="small" onClick={() => prefixLines('> ')}>Quote</Button>
        <Button size="small" onClick={insertLink}>Link</Button>
        <Button size="small" onClick={() => fileRef.current?.click()}>Image</Button>
        <Button size="small" onClick={() => insertText('\n| Column | Column |\n| --- | --- |\n| Value | Value |\n')}>Table</Button>
        <Button size="small" onClick={() => insertText('\n```text\ncode\n```\n', 9)}>Block</Button>
        {imageSuggestions.length > 0 && <Select
          size="small"
          className="markdownAssetSelect"
          placeholder="Uploads"
          value={undefined}
          options={imageSuggestions.map(url => ({ label: url, value: url }))}
          onChange={insertImage}
        />}
      </Space>
      <input
        ref={fileRef}
        type="file"
        accept="image/*"
        hidden
        onChange={event => {
          const file = event.target.files?.[0]
          event.currentTarget.value = ''
          if (file) uploadAndInsert(file).catch(() => undefined)
        }}
      />
    </div>
    <div ref={hostRef} className="markdownCodeMirror" />
  </div>
}
