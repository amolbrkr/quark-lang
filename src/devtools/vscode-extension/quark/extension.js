// Quark language support: run command, completion and hover.
//
// Keywords, builtins, methods and stdlib modules come from data/quark.json,
// which is generated from the compiler (see src/core/quark/internal/editordata).
// Everything else is read from the open document with simple text scans; a
// language server would replace those.

const fs = require('fs');
const path = require('path');
const vscode = require('vscode');

const DATA = require('./data/quark.json');

const IDENT = '[A-Za-z_][A-Za-z0-9_]*';
const KEYWORDS = new Set(DATA.keywords);
const FUNCTIONS = new Map(DATA.functions.map((f) => [f.name, f]));
const STDLIB = new Map(DATA.stdlib.map((m) => [m.name, m]));

// Python and R spellings that aren't Quark, with what to write instead.
// The grammar highlights the same words as invalid.
const HABITS = {
	def: 'Quark defines functions with `fn`: `fn name(x) -> x + 1`',
	elif: 'Quark uses `elseif`',
	return: 'Quark has no `return`: a function evaluates to its last expression',
	True: 'Quark booleans are lowercase: `true`',
	False: 'Quark booleans are lowercase: `false`',
	None: 'Quark uses `null`',
	TRUE: 'Quark booleans are lowercase: `true`',
	FALSE: 'Quark booleans are lowercase: `false`',
	NULL: 'Quark uses `null`',
	function: 'Quark functions are `fn(x) -> x + 1` (anonymous) or `fn name(x) -> ...`',
	'&&': 'Quark uses `and`',
	'||': 'Quark uses `or`',
};

// ---------------------------------------------------------------------------
// Run

// resolveExecutable returns the quark compiler to run: the configured path if
// the user set one, else a compiler built in an open Quark checkout, else
// `quark` from PATH.
function resolveExecutable(uri) {
	const config = vscode.workspace.getConfiguration('quark', uri);
	const inspected = config.inspect('executablePath') || {};
	const explicit = inspected.workspaceFolderValue || inspected.workspaceValue || inspected.globalValue;
	if (explicit) {
		return explicit;
	}
	for (const folder of vscode.workspace.workspaceFolders || []) {
		for (const name of ['quark.exe', 'quark']) {
			const candidate = path.join(folder.uri.fsPath, 'src', 'core', 'quark', name);
			if (fs.existsSync(candidate)) {
				return candidate;
			}
		}
	}
	return 'quark';
}

function quoteShellArg(value) {
	return `"${String(value).replace(/(["\\$`])/g, '\\$1')}"`;
}

async function runCurrentFile() {
	const editor = vscode.window.activeTextEditor;
	if (!editor) {
		vscode.window.showErrorMessage('No active editor. Open a .qrk file to run.');
		return;
	}
	const document = editor.document;
	if (document.languageId !== 'quark' && !document.fileName.toLowerCase().endsWith('.qrk')) {
		vscode.window.showErrorMessage('Active file is not a Quark file (.qrk).');
		return;
	}
	if (document.isUntitled) {
		vscode.window.showErrorMessage('Save the file before running it.');
		return;
	}
	if (!(await document.save())) {
		vscode.window.showErrorMessage('Could not save the file before running it.');
		return;
	}

	const exe = resolveExecutable(document.uri);
	const terminalName = 'Quark';
	let terminal = vscode.window.terminals.find((t) => t.name === terminalName);
	if (!terminal) {
		terminal = vscode.window.createTerminal({ name: terminalName });
	}
	const command = `${quoteShellArg(exe)} run ${quoteShellArg(document.fileName)}`;
	terminal.show(true);
	// PowerShell needs & to run a quoted path.
	terminal.sendText(process.platform === 'win32' ? `& ${command}` : command, true);
}

// ---------------------------------------------------------------------------
// Document scanning

// inCommentOrString reports whether the end of linePrefix is inside a //
// comment or an unterminated string.
function inCommentOrString(linePrefix) {
	let quote = null;
	for (let i = 0; i < linePrefix.length; i += 1) {
		const ch = linePrefix[i];
		if (quote) {
			if (ch === '\\') {
				i += 1;
			} else if (ch === quote) {
				quote = null;
			}
		} else if (ch === "'" || ch === '"') {
			quote = ch;
		} else if (ch === '/' && linePrefix[i + 1] === '/') {
			return true;
		}
	}
	return quote !== null;
}

function documentLines(document, untilLine) {
	const end = untilLine === undefined ? document.lineCount - 1 : Math.min(untilLine, document.lineCount - 1);
	const lines = [];
	for (let i = 0; i <= end; i += 1) {
		lines.push(document.lineAt(i).text);
	}
	return lines;
}

// moduleFunctionsFromText lists the functions defined inside `module name:`
// blocks, keyed by module name. A `//` comment line above a function is its doc.
function moduleFunctionsFromText(text) {
	const modules = new Map();
	const lines = text.split(/\r?\n/);
	let current = null;
	let currentIndent = -1;
	for (let i = 0; i < lines.length; i += 1) {
		const line = lines[i];
		if (!line.trim() || line.trim().startsWith('//')) {
			continue;
		}
		const indent = line.match(/^\s*/)[0].length;
		const mod = line.match(new RegExp(`^(\\s*)module\\s+(${IDENT})\\s*:`));
		if (mod) {
			current = { name: mod[2], functions: [] };
			currentIndent = mod[1].length;
			modules.set(current.name, current);
			continue;
		}
		if (current && indent <= currentIndent) {
			current = null;
		}
		if (!current) {
			continue;
		}
		const fn = line.match(new RegExp(`^\\s+fn\\s+(${IDENT})\\s*\\(([^)]*)\\)\\s*(${IDENT})?\\s*->`));
		if (fn) {
			const prev = i > 0 ? lines[i - 1].trim() : '';
			current.functions.push({
				name: fn[1],
				signature: `${fn[1]}(${fn[2].trim()})${fn[3] ? ` ${fn[3]}` : ''}`,
				doc: prev.startsWith('//') ? prev.replace(/^\/\/\s*/, '') : undefined,
			});
		}
	}
	return modules;
}

// structFields lists the fields of each `struct Name:` block in text.
function structFields(text) {
	const structs = new Map();
	const lines = text.split(/\r?\n/);
	for (let i = 0; i < lines.length; i += 1) {
		const m = lines[i].match(new RegExp(`^(\\s*)struct\\s+(${IDENT})\\s*:`));
		if (!m) {
			continue;
		}
		const fields = [];
		for (let j = i + 1; j < lines.length; j += 1) {
			const line = lines[j];
			if (!line.trim() || line.trim().startsWith('//')) {
				continue;
			}
			if (line.match(/^\s*/)[0].length <= m[1].length) {
				break;
			}
			const f = line.match(new RegExp(`^\\s+(${IDENT})\\s*:\\s*([^=]+?)\\s*(=.*)?$`));
			if (f) {
				fields.push({ name: f[1], type: f[2] });
			}
		}
		structs.set(m[2], fields);
	}
	return structs;
}

// importedModules maps each name usable before a dot to the module behind
// it: `use 'std/io' as io`, `use './other' as o`, `use name` (same file).
function importedModules(document) {
	const text = document.getText();
	const result = new Map();
	const local = moduleFunctionsFromText(text);
	for (const [name, mod] of local) {
		result.set(name, { name, functions: mod.functions });
	}
	const re = new RegExp(`^\\s*use\\s+(?:['"]([^'"]+)['"]|(${IDENT}))(?:\\s+as\\s+(${IDENT}))?`, 'gm');
	let m;
	while ((m = re.exec(text)) !== null) {
		const [, importPath, sameFile, alias] = m;
		if (sameFile) {
			const mod = local.get(sameFile);
			if (mod && alias) {
				result.set(alias, { name: sameFile, functions: mod.functions });
			}
			continue;
		}
		const mod = loadImport(importPath, document);
		if (mod) {
			result.set(alias || mod.name, mod);
		}
	}
	return result;
}

function loadImport(importPath, document) {
	if (importPath.startsWith('std/')) {
		const mod = STDLIB.get(importPath.slice(4).replace(/\.qrk$/, ''));
		return mod ? { name: mod.name, functions: mod.functions } : undefined;
	}
	if (document.isUntitled) {
		return undefined;
	}
	let file = path.resolve(path.dirname(document.fileName), importPath);
	if (!path.extname(file)) {
		file += '.qrk';
	}
	try {
		const modules = moduleFunctionsFromText(fs.readFileSync(file, 'utf8'));
		const first = modules.values().next().value;
		return first ? { name: first.name, functions: first.functions } : undefined;
	} catch (e) {
		return undefined;
	}
}

// ---------------------------------------------------------------------------
// Receiver types for method completion

function typeOfExpression(expr, known, structs) {
	const text = String(expr || '').trim();
	if (!text) {
		return undefined;
	}
	const ctor = text.match(new RegExp(`^(${IDENT})\\s*\\{`));
	if (ctor && structs.has(ctor[1])) {
		return `struct:${ctor[1]}`;
	}
	if (/^['"]/.test(text) || /^(to_str|input)\s*\(/.test(text)) return 'str';
	if (/^list\s*\[/.test(text) || /^(range|enumerate)\s*\(/.test(text)) return 'list';
	if (/^dict\s*\{/.test(text)) return 'dict';
	if (/^vector\s*\[/.test(text)) return 'vector';
	if (/\.(split|to_list|keys|values|items)\s*\([^()]*\)\s*$/.test(text)) return 'list';
	if (/\.(to_vector|astype|fillna)\s*\([^()]*\)\s*$/.test(text)) return 'vector';
	if (/\.(upper|lower|trim|replace|concat)\s*\([^()]*\)\s*$/.test(text)) return known[text.split('.')[0]] === 'list' ? 'list' : 'str';
	const ident = text.match(new RegExp(`^(${IDENT})$`));
	if (ident) return known[ident[1]];
	return undefined;
}

function annotationType(annotation, structs) {
	const t = annotation.trim();
	if (['str', 'list', 'dict', 'vector'].includes(t)) return t;
	if (structs.has(t)) return `struct:${t}`;
	return undefined;
}

// knownTypes scans the document up to line for `x: T` annotations
// (parameters, typed declarations) and `x = <expr>` assignments.
function knownTypes(lines, structs) {
	const known = {};
	const annotated = new RegExp(`(${IDENT})\\s*:\\s*(${IDENT})`, 'g');
	for (const line of lines) {
		if (/^\s*struct\b/.test(line)) {
			continue;
		}
		let m;
		while ((m = annotated.exec(line)) !== null) {
			const t = annotationType(m[2], structs);
			if (t) known[m[1]] = t;
		}
		const assign = line.match(new RegExp(`^\\s*(${IDENT})\\s*(?::\\s*${IDENT}\\s*)?=(?!=)\\s*(.+)$`));
		if (assign) {
			const t = typeOfExpression(assign[2], known, structs);
			if (t) known[assign[1]] = t;
		}
	}
	return known;
}

// receiverType returns the type of the expression before a trailing dot in
// linePrefix (which ends with "<receiver>.<partial word>").
function receiverType(linePrefix, known, structs) {
	const before = linePrefix.replace(/\.\s*[A-Za-z0-9_]*$/, '');
	if (/(['"])(?:\\.|(?!\1).)*\1\s*$/.test(before)) return 'str';
	if (/\blist\s*\[[^\]]*\]\s*$/.test(before)) return 'list';
	if (/\bvector\s*\[[^\]]*\]\s*$/.test(before)) return 'vector';
	if (/\bdict\s*\{[^}]*\}\s*$/.test(before)) return 'dict';
	const call = before.match(/\.([A-Za-z_][A-Za-z0-9_]*)\s*\([^()]*\)\s*$/);
	if (call) return typeOfExpression(before.trim(), known, structs);
	const ident = before.match(new RegExp(`(${IDENT})\\s*$`));
	return ident ? known[ident[1]] : undefined;
}

// ---------------------------------------------------------------------------
// Completion

function functionItem(fn, kind, detailPrefix) {
	const item = new vscode.CompletionItem(fn.name, kind);
	item.detail = detailPrefix ? `${detailPrefix} ${fn.signature}` : fn.signature;
	if (fn.doc) {
		item.documentation = new vscode.MarkdownString(fn.doc);
	}
	item.insertText = new vscode.SnippetString(`${fn.name}($0)`);
	item.command = { command: 'editor.action.triggerParameterHints', title: 'Parameter hints' };
	return item;
}

function dotCompletions(document, position, linePrefix) {
	const lines = documentLines(document, position.line);
	const text = document.getText();
	const structs = structFields(text);

	const receiverName = linePrefix.match(new RegExp(`(?:^|[^.\\w])(${IDENT})\\s*\\.\\s*[A-Za-z0-9_]*$`));
	if (receiverName) {
		const mod = importedModules(document).get(receiverName[1]);
		if (mod) {
			return mod.functions.map((fn) => functionItem(fn, vscode.CompletionItemKind.Function, `${mod.name}.`));
		}
	}

	const type = receiverType(linePrefix, knownTypes(lines, structs), structs);
	if (type && type.startsWith('struct:')) {
		const name = type.slice('struct:'.length);
		return (structs.get(name) || []).map((f) => {
			const item = new vscode.CompletionItem(f.name, vscode.CompletionItemKind.Field);
			item.detail = `${name}.${f.name}: ${f.type}`;
			return item;
		});
	}
	if (type && DATA.methods[type]) {
		return DATA.methods[type].map((fn) => functionItem(fn, vscode.CompletionItemKind.Method, `${type}.`));
	}
	// Unknown receiver: offer every method, saying which types have it.
	const byName = new Map();
	for (const [recv, methods] of Object.entries(DATA.methods)) {
		for (const fn of methods) {
			if (!byName.has(fn.name)) byName.set(fn.name, { fn, types: [] });
			byName.get(fn.name).types.push(recv);
		}
	}
	return Array.from(byName.values()).map(({ fn, types }) => {
		const item = functionItem(fn, vscode.CompletionItemKind.Method);
		item.detail = `${types.join(' | ')} method: ${fn.signature}`;
		return item;
	});
}

function generalCompletions(document, position) {
	const items = [];
	for (const kw of DATA.keywords) {
		items.push(new vscode.CompletionItem(kw, vscode.CompletionItemKind.Keyword));
	}
	for (const fn of DATA.functions) {
		items.push(functionItem(fn, vscode.CompletionItemKind.Function, 'builtin'));
	}
	for (const name of importedModules(document).keys()) {
		const item = new vscode.CompletionItem(name, vscode.CompletionItemKind.Module);
		item.detail = 'module';
		items.push(item);
	}
	const seen = new Set([...KEYWORDS, ...FUNCTIONS.keys()]);
	const text = documentLines(document).join('\n').replace(/\/\/.*$/gm, '').replace(/(['"])(?:\\.|(?!\1).)*\1/g, '');
	for (const id of text.match(/\b[A-Za-z_][A-Za-z0-9_]*\b/g) || []) {
		if (!seen.has(id) && !HABITS[id]) {
			seen.add(id);
			items.push(new vscode.CompletionItem(id, vscode.CompletionItemKind.Variable));
		}
	}
	return items;
}

async function importCompletions(document) {
	const items = DATA.stdlib.map((m) => {
		const item = new vscode.CompletionItem(`std/${m.name}`, vscode.CompletionItemKind.Module);
		item.detail = m.functions.map((f) => f.name).join(', ');
		return item;
	});
	if (document.isUntitled) {
		return items;
	}
	const dir = path.dirname(document.fileName);
	const files = await vscode.workspace.findFiles('**/*.qrk', '**/{.git,node_modules,deps}/**', 500);
	for (const uri of files) {
		if (uri.fsPath === document.uri.fsPath) {
			continue;
		}
		let rel = path.relative(dir, uri.fsPath).replace(/\\/g, '/').replace(/\.qrk$/, '');
		if (!rel.startsWith('../')) {
			rel = `./${rel}`;
		}
		items.push(new vscode.CompletionItem(rel, vscode.CompletionItemKind.File));
	}
	return items;
}

const completionProvider = {
	provideCompletionItems(document, position, token, context) {
		const linePrefix = document.lineAt(position.line).text.slice(0, position.character);
		if (/^\s*use\s+['"][^'"]*$/.test(linePrefix)) {
			return importCompletions(document);
		}
		// Quotes and / only trigger completion for import paths.
		const trigger = context && context.triggerCharacter;
		if (trigger && trigger !== '.') {
			return [];
		}
		if (inCommentOrString(linePrefix)) {
			return [];
		}
		if (/\.\s*[A-Za-z0-9_]*$/.test(linePrefix) && !/\b\d+\.[0-9]*$/.test(linePrefix)) {
			return dotCompletions(document, position, linePrefix);
		}
		return generalCompletions(document, position);
	},
};

// ---------------------------------------------------------------------------
// Hover

const hoverProvider = {
	provideHover(document, position) {
		const line = document.lineAt(position.line).text;
		const hash = line.indexOf('#');
		if (hash >= 0 && position.character >= hash && !inCommentOrString(line.slice(0, hash))) {
			return new vscode.Hover('Quark comments start with `//`');
		}
		const op = document.getWordRangeAtPosition(position, /&&|\|\|/);
		if (op) {
			return new vscode.Hover(HABITS[document.getText(op)], op);
		}
		const range = document.getWordRangeAtPosition(position);
		if (!range) {
			return undefined;
		}
		const word = document.getText(range);
		const prefix = line.slice(0, range.start.character);
		if (inCommentOrString(prefix)) {
			return undefined;
		}
		if (HABITS[word]) {
			return new vscode.Hover(new vscode.MarkdownString(HABITS[word]), range);
		}
		const md = (sig, doc) => {
			const s = new vscode.MarkdownString();
			s.appendCodeblock(sig, 'quark');
			if (doc) s.appendMarkdown(doc);
			return s;
		};
		const qualified = prefix.match(new RegExp(`(${IDENT})\\s*\\.\\s*$`));
		if (qualified) {
			const mod = importedModules(document).get(qualified[1]);
			const fn = mod && mod.functions.find((f) => f.name === word);
			if (fn) {
				return new vscode.Hover(md(`${mod.name}.${fn.signature}`, fn.doc), range);
			}
			const sigs = [];
			for (const [recv, methods] of Object.entries(DATA.methods)) {
				for (const m of methods) {
					if (m.name === word) sigs.push(`${recv}.${m.signature}`);
				}
			}
			if (sigs.length > 0) {
				return new vscode.Hover(md(sigs.join('\n')), range);
			}
			return undefined;
		}
		const fn = FUNCTIONS.get(word);
		if (fn && /^\s*\(/.test(line.slice(range.end.character))) {
			return new vscode.Hover(md(fn.signature, 'builtin'), range);
		}
		return undefined;
	},
};

// ---------------------------------------------------------------------------

function activate(context) {
	context.subscriptions.push(
		vscode.commands.registerCommand('quark.runCurrentFile', runCurrentFile),
		vscode.languages.registerCompletionItemProvider('quark', completionProvider, '.', "'", '"', '/'),
		vscode.languages.registerHoverProvider('quark', hoverProvider),
	);
}

function deactivate() {}

module.exports = {
	activate,
	deactivate,
	// For tests.
	_internal: { inCommentOrString, moduleFunctionsFromText, structFields, knownTypes, receiverType },
};
