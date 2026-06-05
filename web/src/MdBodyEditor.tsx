import { useEffect, useRef } from 'react'
import { Select } from 'antd'
import { CrepeBuilder } from '@milkdown/crepe/builder'
import { blockEdit } from '@milkdown/crepe/feature/block-edit'
import { cursor } from '@milkdown/crepe/feature/cursor'
import { imageBlock } from '@milkdown/crepe/feature/image-block'
import { linkTooltip } from '@milkdown/crepe/feature/link-tooltip'
import { listItem } from '@milkdown/crepe/feature/list-item'
import { placeholder } from '@milkdown/crepe/feature/placeholder'
import { table } from '@milkdown/crepe/feature/table'
import { toolbar } from '@milkdown/crepe/feature/toolbar'
import type { ListenerManager } from '@milkdown/kit/plugin/listener'
import { insert, replaceAll } from '@milkdown/kit/utils'
import '@milkdown/crepe/theme/common/prosemirror.css'
import '@milkdown/crepe/theme/common/reset.css'
import '@milkdown/crepe/theme/common/block-edit.css'
import '@milkdown/crepe/theme/common/cursor.css'
import '@milkdown/crepe/theme/common/image-block.css'
import '@milkdown/crepe/theme/common/link-tooltip.css'
import '@milkdown/crepe/theme/common/list-item.css'
import '@milkdown/crepe/theme/common/placeholder.css'
import '@milkdown/crepe/theme/common/table.css'
import '@milkdown/crepe/theme/common/toolbar.css'
import '@milkdown/crepe/theme/frame.css'

type MdBodyEditorProps = {
  adminDark: boolean
  editorKey: string
  imageSuggestions: string[]
  markdown: string
  onChange: (value: string) => void
  uploadImage: (file: File) => Promise<string>
}

export default function MdBodyEditor({ adminDark, editorKey, imageSuggestions, markdown, onChange, uploadImage }: MdBodyEditorProps) {
  const hostRef = useRef<HTMLDivElement | null>(null)
  const editorRef = useRef<CrepeBuilder | null>(null)
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
      .addFeature(imageBlock, {
        onUpload: (file: File) => uploadImageRef.current(file),
        inlineOnUpload: (file: File) => uploadImageRef.current(file),
        blockOnUpload: (file: File) => uploadImageRef.current(file)
      })
      .addFeature(blockEdit)
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
      })
      .catch(() => undefined)

    return () => {
      cancelled = true
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
