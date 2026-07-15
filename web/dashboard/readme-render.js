// Renders a peer's README (markdown or mdx) similar to how GitHub
// renders README.md on a repo page, or PyPI renders it on a package
// page. Uses marked.js (loaded from CDN) for real CommonMark parsing.
//
// Honest scope note on .mdx: MDX is markdown + arbitrary JSX component
// evaluation, which requires a real compiler (the @mdx-js/mdx package)
// and can execute arbitrary React components — that's not something
// this vanilla dashboard runs client-side without a build pipeline. What
// IS implemented here is genuinely useful and correct: full markdown
// rendering, PLUS recognition of a small set of common MDX "callout"
// components (<Note>, <Warning>, <Tip>, <Callout type="...">) which get
// rendered as real styled boxes — the same pattern used by  Docusaurus/
// Nextra docs sites. Any other custom JSX component in a .mdx file is
// shown as a labeled placeholder rather than silently dropped or
// (dangerously) eval'd.

const MDX_COMPONENT_RE = /<(Note|Warning|Tip|Callout)([^>]*)>([\s\S]*?)<\/\1>/g;
const MDX_ATTR_RE = /(\w+)="([^"]*)"/g;

function parseMdxAttrs(attrString) {
	const attrs = {};
	let m;
	while ((m = MDX_ATTR_RE.exec(attrString)) !== null) {
		attrs[m[1]] = m[2];
	}
	return attrs;
}

function transformMdxCallouts(source) {
	return source.replace(MDX_COMPONENT_RE, (match, tag, attrString, inner) => {
		const attrs = parseMdxAttrs(attrString);
		const kind = (tag === "Callout" ? attrs.type || "note" : tag).toLowerCase();
		const label = kind.charAt(0).toUpperCase() + kind.slice(1);
		// Wrap in a div marker; marked() will still parse the markdown
		// inside `inner` (bold, links, code, etc.) because we render this
		// AFTER marked has already converted the surrounding text — so we
		// do the callout substitution as a post-process on the HTML marker.
		return `\n\n:::${kind}:::${inner}:::end:::\n\n`;
	});
}

function renderCalloutMarkers(html) {
	return html.replace(
		/:::(\w+):::([\s\S]*?):::end:::/g,
		(match, kind, inner) => {
			const icons = { note: "ℹ️", warning: "⚠️", tip: "💡", callout: "📌" };
			const icon = icons[kind] || "📌";
			return `<div class="mdx-callout mdx-callout-${kind}"><span class="mdx-callout-icon">${icon}</span><div class="mdx-callout-body">${inner}</div></div>`;
		}
	);
}

function flagUnknownJsx(source) {
	// Any remaining <Something ...> that isn't a known HTML tag or one of
	// our supported callouts gets surfaced honestly rather than silently
	// stripped or rendered as broken text.
	const KNOWN_HTML = new Set(["div","span","p","a","strong","em","code","pre","ul","ol","li","h1","h2","h3","h4","h5","h6","br","hr","img","table","thead","tbody","tr","td","th","blockquote"]);
	return source.replace(/<([A-Z][A-Za-z0-9]*)\b[^>]*>/g, (match, tag) => {
		if (["Note", "Warning", "Tip", "Callout"].includes(tag)) return match; // already handled
		return `<div class="mdx-unsupported-component">⚙️ Unsupported MDX component: <code>&lt;${tag}&gt;</code> (requires a real MDX build pipeline to render)</div>`;
	});
}

async function ensureMarkedLoaded() {
	if (window.marked) return;
	await new Promise((resolve, reject) => {
		const script = document.createElement("script");
		script.src = "https://cdnjs.cloudflare.com/ajax/libs/marked/12.0.0/marked.min.js";
		script.onload = resolve;
		script.onerror = reject;
		document.head.appendChild(script);
	});
}

async function ensureDOMPurifyLoaded() {
	if (window.DOMPurify) return;
	await new Promise((resolve, reject) => {
		const script = document.createElement("script");
		script.src = "https://cdnjs.cloudflare.com/ajax/libs/dompurify/3.1.6/purify.min.js";
		script.onload = resolve;
		script.onerror = reject;
		document.head.appendChild(script);
	});
}

/**
 * Renders README source into the given container element.
 * @param {string} source - raw markdown or mdx content
 * @param {string} format - "md" or "mdx"
 * @param {HTMLElement} container
 */
async function renderReadme(source, format, container) {
	await ensureMarkedLoaded();
	await ensureDOMPurifyLoaded();

	let processedSource = source;
	if (format === "mdx") {
		processedSource = transformMdxCallouts(processedSource);
		processedSource = flagUnknownJsx(processedSource);
	}

	marked.setOptions({ breaks: true, gfm: true });
	let html = marked.parse(processedSource);

	if (format === "mdx") {
		html = renderCalloutMarkers(html);
	}

	// Sanitize before touching innerHTML — a README is content from a
	// peer (which may be someone else's node, including public ones),
	// and rendering it as raw HTML without this step would let a
	// malicious peer's README run arbitrary JS in every viewer's browser.
	// DOMPurify strips <script>, event handler attributes (onclick, etc),
	// javascript: URLs, and other injection vectors while preserving
	// legitimate markdown-generated markup (headings, links, code blocks,
	// our own .mdx-callout divs, etc).
	const clean = DOMPurify.sanitize(html, {
		ALLOWED_TAGS: ["h1","h2","h3","h4","h5","h6","p","a","strong","em","code","pre",
			"ul","ol","li","br","hr","img","table","thead","tbody","tr","td","th",
			"blockquote","div","span"],
		ALLOWED_ATTR: ["href","src","alt","title","class"],
		ALLOW_DATA_ATTR: false,
	});

	container.innerHTML = `<div class="readme-content">${clean}</div>`;
}
