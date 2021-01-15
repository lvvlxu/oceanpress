package main

import (
	"bytes"
	"html/template"
	"path"
	"strings"

	"github.com/2234839/md2website/src/util"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
)

// LuteEngine lute 实例
var LuteEngine = lute.New()

/** 用于从 md 文档中解析获得一些结构性信息 */
var mdStructuredLuteEngine = lute.New()

/** 当前被处理的 entity */
var baseEntity FileEntity

func init() {
	/** 对引用块进行渲染 */
	LuteEngine.SetBlockRef(true)
	// /** 渲染 id （渲染为空） */
	LuteEngine.SetKramdownIAL(true)
	// /** 标题的链接 a 标签渲染 */
	LuteEngine.SetHeadingAnchor(true)
	LuteEngine.SetKramdownIALIDRenderName("data-block-id")

	mdStructuredLuteEngine.SetKramdownIAL(true)
	mdStructuredLuteEngine.SetKramdownIALIDRenderName("data-block-id")
	// 当前正在被渲染的块的 id
	var id string
	/** 获取块的id */
	var getBlockID = func(n *ast.Node, _ bool) (string, ast.WalkStatus) {
		id = n.TokensStr()
		return "", ast.WalkContinue
	}
	LuteEngine.Md2HTMLRendererFuncs[ast.NodeBlockRefID] = getBlockID
	LuteEngine.Md2HTMLRendererFuncs[ast.NodeBlockEmbedID] = getBlockID

	/** 块引用渲染,类似于超链接 */
	LuteEngine.Md2HTMLRendererFuncs[ast.NodeBlockRefText] = func(n *ast.Node, entering bool) (string, ast.WalkStatus) {
		var html string
		if entering {
			fileEntity, mdInfo, err := FindFileEntityFromID(id)
			if err != nil {
				return "", ast.WalkContinue
			}
			var src string
			if fileEntity.path != "" {
				src = FileEntityRelativePath(baseEntity, fileEntity, id)
			}
			var title = n.Text()

			// 锚文本模板变量处理 使用定义块内容文本填充。如定义块是文档块，则使用文档名填充。
			if title == "{{.text}}" {
				if mdInfo.blockType == "NodeDocument" {
					title = fileEntity.name
				} else {
					title = mdInfo.node.Text()
				}
			}
			html = BlockRefRender(BlockRefInfo{
				Src:   src,
				Title: title,
			})
		}
		return html, ast.WalkSkipChildren
	}
	/** 嵌入块渲染 */
	LuteEngine.Md2HTMLRendererFuncs[ast.NodeBlockEmbedText] = func(n *ast.Node, entering bool) (string, ast.WalkStatus) {

		var html string
		if entering {
			fileEntity, mdInfo, err := FindFileEntityFromID(id)
			if err != nil {
				return "", ast.WalkContinue
			}
			var src string
			if fileEntity.path != "" {
				src = FileEntityRelativePath(baseEntity, fileEntity, id)
			}
			// 修改 base 路径以使用 ../ 这样的形式指向根目录再深入到待解析的md文档所在的路径 ,就在下面一点点会再重置回去
			LuteEngine.RenderOptions.LinkBase = strings.Repeat("../", strings.Count(baseEntity.relativePath, "/")-1) + "." + path.Dir(fileEntity.relativePath)
			html = EmbeddedBlockRender(EmbeddedBlockInfo{
				Src:   src,
				Title: n.Text(),
				// 这里涉及到一个套娃问题，还有 baseEntity 该怎么处理。以及他们的路径怎么办
				Content: template.HTML(LuteEngine.MarkdownStr("", renderNodeMarkdown(mdInfo.node))),
			})
			LuteEngine.RenderOptions.LinkBase = ""
		}
		return html, ast.WalkSkipChildren
	}

}

// MdStructInfo md 结构信息
type MdStructInfo struct {
	blockID   string
	blockType string
	mdContent string
	node      *ast.Node
}

// GetMdStructInfo 从 md 获取结构信息
func GetMdStructInfo(name string, md string) []MdStructInfo {

	luteEngine := mdStructuredLuteEngine
	tree := parse.Parse(name, []byte(md), luteEngine.ParseOptions)

	var infoList []MdStructInfo
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if entering {
			return ast.WalkContinue
		}

		if nil == n.FirstChild {
			return ast.WalkSkipChildren
		}
		content := renderBlockMarkdown(n)
		if strings.Contains(n.Text(), "岁，一事无成，未来还有希望吗？") {
			var id = n.IALAttr("id")
			util.Log(id)
			util.Log(22)
		}
		infoList = append(infoList, MdStructInfo{
			blockID:   n.IALAttr("id"),
			blockType: n.Type.String(),
			mdContent: content,
			node:      n,
		})

		return ast.WalkContinue
	})
	return infoList
}

func renderBlockMarkdown(node *ast.Node) string {
	root := &ast.Node{Type: ast.NodeDocument}
	luteEngine := mdStructuredLuteEngine

	tree := &parse.Tree{Root: root, Context: &parse.Context{ParseOption: luteEngine.ParseOptions}}
	renderer := render.NewFormatRenderer(tree, luteEngine.RenderOptions)
	renderer.Writer = &bytes.Buffer{}
	renderer.NodeWriterStack = append(renderer.NodeWriterStack, renderer.Writer)
	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		rendererFunc := renderer.RendererFuncs[n.Type]
		return rendererFunc(n, entering)
	})
	return strings.TrimSpace(renderer.Writer.String())
}

// FileEntityToHTML 转 html
func FileEntityToHTML(entity FileEntity) string {
	baseEntity = entity
	return LuteEngine.MarkdownStr("", entity.mdStr)
}

// FileEntityRelativePath 计算他们变成 html 文件之后的相对路径
func FileEntityRelativePath(base FileEntity, cur FileEntity, id string) string {
	// 减一是因为 路径开头必有 / 而这里只需要跳到这一层
	count := strings.Count(base.relativePath, "/")
	if strings.HasPrefix(base.relativePath, "/") {
		count--
	}
	l2 := strings.Split(cur.relativePath, "/")
	url := strings.Repeat("../", count)
	url += strings.Join(l2[1:], "/")
	url = FilePathToWebPath(url)
	url += "#" + id
	return url
}

// FilePathToWebPath 将相对文件路径转为 web路径，主要是去除文件中的id 以及添加 .html
func FilePathToWebPath(filePath string) string {
	if strings.HasSuffix(filePath, ".md") {
		return filePath[0:len(filePath)-3] + ".html"
	}
	// 大概率是空
	return filePath
}

// 将 Node 渲染为 md 对于 header 节点特殊处理，会将他的 child 包含进来
func renderNodeMarkdown(node *ast.Node) string {
	// 收集块
	var nodes []*ast.Node
	ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
		if entering {
			nodes = append(nodes, n)
			if ast.NodeHeading == node.Type {
				// 支持“标题块”引用
				children := headingChildren(n)
				nodes = append(nodes, children...)
			}
		}
		return ast.WalkSkipChildren
	})

	// 渲染块
	root := &ast.Node{Type: ast.NodeDocument}
	luteEngine := lute.New()
	tree := &parse.Tree{Root: root, Context: &parse.Context{ParseOption: luteEngine.ParseOptions}}
	tree.Context.ParseOption.KramdownIAL = false // 关闭 IAL
	renderer := render.NewFormatRenderer(tree, luteEngine.RenderOptions)
	renderer.Writer = &bytes.Buffer{}
	renderer.NodeWriterStack = append(renderer.NodeWriterStack, renderer.Writer) // 因为有可能不是从 root 开始渲染，所以需要初始化
	for _, node := range nodes {
		ast.Walk(node, func(n *ast.Node, entering bool) ast.WalkStatus {
			rendererFunc := renderer.RendererFuncs[n.Type]
			return rendererFunc(n, entering)
		})
	}
	return strings.TrimSpace(renderer.Writer.String())
}

func headingChildren(heading *ast.Node) (ret []*ast.Node) {
	currentLevel := heading.HeadingLevel
	var blocks []*ast.Node
	for n := heading.Next; nil != n; n = n.Next {
		if ast.NodeHeading == n.Type {
			if currentLevel >= n.HeadingLevel {
				break
			}
		}
		blocks = append(blocks, n)
	}
	ret = append(ret, blocks...)
	return
}
