import { MDXEditor, headingsPlugin, listsPlugin, quotePlugin, thematicBreakPlugin, markdownShortcutPlugin, toolbarPlugin, UndoRedo, BoldItalicUnderlineToggles, ListsToggle, BlockTypeSelect, CreateLink, InsertImage, imagePlugin, linkDialogPlugin, linkPlugin, codeBlockPlugin, codeMirrorPlugin, InsertCodeBlock, CodeToggle, ConditionalContents, ChangeCodeMirrorLanguage, Separator, InsertTable, tablePlugin, InsertThematicBreak } from '@mdxeditor/editor'
import '@mdxeditor/editor/style.css'

type MdxBodyEditorProps = {
  adminDark: boolean
  editorKey: string
  imageSuggestions: string[]
  markdown: string
  onChange: (value: string) => void
  uploadImage: (file: File) => Promise<string>
}

export default function MdxBodyEditor({ adminDark, editorKey, imageSuggestions, markdown, onChange, uploadImage }: MdxBodyEditorProps) {
  return <MDXEditor
    key={editorKey}
    className={adminDark ? 'tinyMdx dark-theme' : 'tinyMdx'}
    contentEditableClassName="tinyMdxContent"
    markdown={markdown}
    onChange={onChange}
    plugins={[
      headingsPlugin(),
      listsPlugin(),
      quotePlugin(),
      thematicBreakPlugin(),
      linkPlugin(),
      linkDialogPlugin(),
      imagePlugin({ imageUploadHandler: uploadImage, imageAutocompleteSuggestions: imageSuggestions.length ? imageSuggestions : ['/uploads/'] }),
      tablePlugin(),
      codeBlockPlugin({ defaultCodeBlockLanguage: 'text' }),
      codeMirrorPlugin({ codeBlockLanguages: { text: 'Plain text', markdown: 'Markdown', python: 'Python', py: 'Python', javascript: 'JavaScript', typescript: 'TypeScript', jsx: 'JSX', tsx: 'TSX', html: 'HTML', css: 'CSS', json: 'JSON', bash: 'Shell', sh: 'Shell', go: 'Go', sql: 'SQL', yaml: 'YAML', yml: 'YAML', mermaid: 'Mermaid diagram' } }),
      markdownShortcutPlugin(),
      toolbarPlugin({toolbarContents: () => <ConditionalContents options={[
        { when: editor => editor?.editorType === 'codeblock', contents: () => <ChangeCodeMirrorLanguage /> },
        { fallback: () => <><UndoRedo /><Separator /><BoldItalicUnderlineToggles /><CodeToggle /><ListsToggle /><BlockTypeSelect /><Separator /><CreateLink /><InsertImage /><Separator /><InsertTable /><InsertThematicBreak /><InsertCodeBlock /></> }
      ]} />})
    ]}
  />
}
