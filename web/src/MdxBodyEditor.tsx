import { useEffect, useRef } from 'react'
import { Select } from 'antd'
import Editor from '@toast-ui/editor'
import Prism from 'prismjs'
import codeSyntaxHighlight from '@toast-ui/editor-plugin-code-syntax-highlight'
import '@toast-ui/editor/dist/toastui-editor.css'
import '@toast-ui/editor/dist/theme/toastui-editor-dark.css'
import 'prismjs/themes/prism-tomorrow.css'
import '@toast-ui/editor-plugin-code-syntax-highlight/dist/toastui-editor-plugin-code-syntax-highlight.css'
import 'prismjs/components/prism-bash.js'
import 'prismjs/components/prism-css.js'
import 'prismjs/components/prism-go.js'
import 'prismjs/components/prism-javascript.js'
import 'prismjs/components/prism-json.js'
import 'prismjs/components/prism-markdown.js'
import 'prismjs/components/prism-python.js'
import 'prismjs/components/prism-sql.js'
import 'prismjs/components/prism-typescript.js'
import 'prismjs/components/prism-yaml.js'

type MdxBodyEditorProps = {
  adminDark: boolean
  editorKey: string
  imageSuggestions: string[]
  markdown: string
  onChange: (value: string) => void
  uploadImage: (file: File) => Promise<string>
}

export default function MdxBodyEditor({ adminDark, editorKey, imageSuggestions, markdown, onChange, uploadImage }: MdxBodyEditorProps) {
  const hostRef = useRef<HTMLDivElement | null>(null)
  const editorRef = useRef<Editor | null>(null)
  const onChangeRef = useRef(onChange)
  const uploadImageRef = useRef(uploadImage)

  useEffect(() => {
    onChangeRef.current = onChange
  }, [onChange])

  useEffect(() => {
    uploadImageRef.current = uploadImage
  }, [uploadImage])

  useEffect(() => {
    if (!hostRef.current) return

    const editor = new Editor({
      el: hostRef.current,
      initialValue: markdown,
      initialEditType: 'wysiwyg',
      previewStyle: 'vertical',
      minHeight: '540px',
      autofocus: false,
      usageStatistics: false,
      theme: adminDark ? 'dark' : undefined,
      plugins: [[codeSyntaxHighlight, { highlighter: Prism }]],
      events: {
        change: () => {
          onChangeRef.current(editor.getMarkdown())
        }
      },
      hooks: {
        addImageBlobHook: (blob, callback) => {
          uploadImageRef.current(blob as File)
            .then(url => callback(url, blob instanceof File ? blob.name : 'image'))
            .catch(() => undefined)
        }
      }
    })

    editorRef.current = editor
    return () => {
      editor.destroy()
      editorRef.current = null
    }
  }, [adminDark, editorKey])

  useEffect(() => {
    const editor = editorRef.current
    if (!editor) return
    if (editor.getMarkdown() !== markdown) {
      editor.setMarkdown(markdown, false)
    }
  }, [markdown])

  function insertImage(url: string) {
    const editor = editorRef.current
    if (!editor) return
    editor.exec('addImage', { imageUrl: url, altText: 'image' })
    editor.focus()
  }

  return <div className={adminDark ? 'toastMarkdownEditor dark-theme' : 'toastMarkdownEditor'}>
    {imageSuggestions.length > 0 && <div className="toastAssetBar">
      <Select
        size="small"
        className="toastAssetSelect"
        placeholder="Insert uploaded image"
        value={undefined}
        options={imageSuggestions.map(url => ({ label: url, value: url }))}
        onChange={insertImage}
      />
    </div>}
    <div ref={hostRef} />
  </div>
}
