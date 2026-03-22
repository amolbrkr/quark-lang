const vscode = require('vscode');

const KEYWORDS = [
	'use',
	'as',
	'module',
	'in',
	'and',
	'or',
	'if',
	'elseif',
	'else',
	'for',
	'while',
	'when',
	'fn',
	'true',
	'false',
	'null',
	'ok',
	'err',
	'list',
	'dict',
	'vector',
	'result',
	'break',
	'continue',
];

const FREE_BUILTINS = [
	'print',
	'println',
	'input',
	'_file_open',
	'_file_read',
	'_file_write',
	'_file_close',
	'_file_seek',
	'_file_exists',
	'len',
	'to_str',
	'to_int',
	'to_float',
	'to_bool',
	'type',
	'is_ok',
	'is_err',
	'unwrap',
	'range',
	'abs',
	'min',
	'max',
	'sum',
	'sqrt',
	'floor',
	'ceil',
	'round',
	'enumerate',
	'vfrom_list',
];

const METHODS_BY_TYPE = {
	str: ['upper', 'lower', 'trim', 'contains', 'startswith', 'endswith', 'replace', 'concat', 'split', 'slice'],
	list: ['concat', 'push', 'pop', 'get', 'set', 'insert', 'remove', 'slice', 'reverse', 'enumerate', 'join', 'to_vector'],
	dict: ['get', 'set', 'keys', 'values', 'items'],
	vector: ['get', 'fillna', 'astype', 'to_list'],
};

const ALL_METHODS = Array.from(new Set(Object.values(METHODS_BY_TYPE).flat()));

const SIMPLE_SNIPPETS = [
	{ label: 'if', body: 'if ${1:condition}:\n\t$0', detail: 'If block' },
	{ label: 'ifelse', body: 'if ${1:condition}:\n\t${2:pass}\nelse:\n\t$0', detail: 'If / else block' },
	{ label: 'forin', body: 'for ${1:item} in ${2:iterable}:\n\t$0', detail: 'For loop' },
	{ label: 'while', body: 'while ${1:condition}:\n\t$0', detail: 'While loop' },
	{ label: 'fn', body: 'fn ${1:name}(${2:args}) ->\n\t$0', detail: 'Function declaration' },
	{ label: 'use', body: 'use "${1:std/io}"$0', detail: 'Module import' },
];

function quoteShellArg(value) {
	return `"${String(value).replace(/"/g, '\\"')}"`;
}

function createRunCommand() {
	return async function runCurrentFile() {
		const editor = vscode.window.activeTextEditor;
		if (!editor) {
			vscode.window.showErrorMessage('No active editor. Open a .qrk file to run.');
			return;
		}

		const document = editor.document;
		const isQuarkDoc = document.languageId === 'quark' || document.fileName.toLowerCase().endsWith('.qrk');
		if (!isQuarkDoc) {
			vscode.window.showErrorMessage('Active file is not a Quark file (.qrk).');
			return;
		}

		if (document.isUntitled) {
			vscode.window.showErrorMessage('Please save the file before running Quark.');
			return;
		}

		const saved = await document.save();
		if (!saved) {
			vscode.window.showErrorMessage('Could not save file before run.');
			return;
		}

		const config = vscode.workspace.getConfiguration('quark', document.uri);
		const executablePath = config.get('executablePath', 'quark');

		const terminalName = 'Quark Run';
		let terminal = vscode.window.terminals.find((t) => t.name === terminalName);
		if (!terminal) {
			terminal = vscode.window.createTerminal({ name: terminalName });
		}

		const command = `${quoteShellArg(executablePath)} run ${quoteShellArg(document.fileName)}`;
		terminal.show(true);
		terminal.sendText(command, true);
	};
}

function makeItem(label, kind, detail) {
	const item = new vscode.CompletionItem(label, kind);
	if (detail) {
		item.detail = detail;
	}
	return item;
}

function inferTypeFromExpression(expr, knownTypes) {
	const text = String(expr || '').trim();
	if (!text) {
		return undefined;
	}

	if (/\.split\s*\(/.test(text)) {
		return 'list_str';
	}
	if (/^list\s*\[/.test(text) || /\.to_list\s*\(/.test(text) || /^range\s*\(/.test(text) || /\.enumerate\s*\(/.test(text)) {
		return 'list';
	}
	if (/^dict\s*\{/.test(text)) {
		return 'dict';
	}
	if (/^vector\s*\[/.test(text) || /\.to_vector\s*\(/.test(text) || /\.astype\s*\(/.test(text) || /\.fillna\s*\(/.test(text)) {
		return 'vector';
	}
	if (/^['"]/.test(text) || /\.trim\s*\(|\.upper\s*\(|\.lower\s*\(|\.replace\s*\(|\.startswith\s*\(|\.endswith\s*\(|\.concat\s*\(/.test(text)) {
		return 'str';
	}

	const ident = text.match(/^([A-Za-z_][A-Za-z0-9_]*)$/);
	if (ident && knownTypes[ident[1]]) {
		return knownTypes[ident[1]];
	}

	return undefined;
}

function inferKnownTypes(document, untilLine) {
	const known = {};
	const end = Math.min(untilLine, document.lineCount - 1);
	for (let i = 0; i <= end; i += 1) {
		const line = document.lineAt(i).text;
		const m = line.match(/^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.+)$/);
		if (!m) {
			continue;
		}
		const name = m[1];
		const expr = m[2];
		const inferred = inferTypeFromExpression(expr, known);
		if (inferred) {
			known[name] = inferred;
		}
	}
	return known;
}

function collectIdentifiers(document, untilLine) {
	const ids = new Set();
	const end = Math.min(untilLine, document.lineCount - 1);
	for (let i = 0; i <= end; i += 1) {
		const line = document.lineAt(i).text;
		const matches = line.match(/\b[A-Za-z_][A-Za-z0-9_]*\b/g);
		if (!matches) {
			continue;
		}
		for (const id of matches) {
			if (!KEYWORDS.includes(id)) {
				ids.add(id);
			}
		}
	}
	return Array.from(ids);
}

function createGeneralCompletionProvider() {
	return {
		provideCompletionItems(document, position) {
			const items = [];
			for (const keyword of KEYWORDS) {
				items.push(makeItem(keyword, vscode.CompletionItemKind.Keyword, 'Quark keyword'));
			}
			for (const builtin of FREE_BUILTINS) {
				const item = makeItem(builtin, vscode.CompletionItemKind.Function, 'Quark builtin');
				item.insertText = `${builtin}($0)`;
				item.insertTextRules = vscode.CompletionItemInsertTextRule.InsertAsSnippet;
				items.push(item);
			}

			const identifiers = collectIdentifiers(document, position.line);
			for (const id of identifiers) {
				items.push(makeItem(id, vscode.CompletionItemKind.Variable, 'In file'));
			}

			for (const snippet of SIMPLE_SNIPPETS) {
				const item = makeItem(snippet.label, vscode.CompletionItemKind.Snippet, snippet.detail);
				item.insertText = new vscode.SnippetString(snippet.body);
				items.push(item);
			}

			return items;
		},
	};
}

function createMethodCompletionProvider() {
	return {
		provideCompletionItems(document, position) {
			const linePrefix = document.lineAt(position.line).text.slice(0, position.character);
			const receiverMatch = linePrefix.match(/([A-Za-z_][A-Za-z0-9_]*)(\[[^\]]*\])?\.\s*$/);

			if (!receiverMatch) {
				return [];
			}

			const receiverName = receiverMatch[1];
			const usedIndexAccess = Boolean(receiverMatch[2]);
			const knownTypes = inferKnownTypes(document, position.line);
			let typeName = knownTypes[receiverName];
			if (usedIndexAccess) {
				if (typeName === 'list_str') {
					typeName = 'str';
				} else if (typeName && typeName.startsWith('list')) {
					typeName = undefined;
				}
			}
			const methods = typeName && METHODS_BY_TYPE[typeName] ? METHODS_BY_TYPE[typeName] : ALL_METHODS;

			return methods.map((methodName) => {
				const item = makeItem(methodName, vscode.CompletionItemKind.Method, typeName ? `${typeName} method` : 'Quark method');
				item.insertText = `${methodName}($0)`;
				item.insertTextRules = vscode.CompletionItemInsertTextRule.InsertAsSnippet;
				return item;
			});
		},
	};
}

async function findImportCandidates(document) {
	const candidates = new Set(['std/io']);

	const stdFiles = await vscode.workspace.findFiles('src/stdlib/**/*.qrk', '**/{.git,node_modules,deps}/**', 500);
	for (const uri of stdFiles) {
		const rel = vscode.workspace.asRelativePath(uri, false).replace(/\\/g, '/');
		const normalized = rel.replace(/^src\/stdlib\//, '').replace(/\.qrk$/, '');
		if (normalized.length > 0) {
			candidates.add(`std/${normalized}`);
		}
	}

	const qrkFiles = await vscode.workspace.findFiles('**/*.qrk', '**/{.git,node_modules,deps}/**', 1000);
	for (const uri of qrkFiles) {
		if (uri.fsPath === document.uri.fsPath) {
			continue;
		}
		const relFromDocDir = vscode.workspace.asRelativePath(uri, false).replace(/\\/g, '/');
		candidates.add(relFromDocDir.replace(/\.qrk$/, ''));
	}

	return Array.from(candidates).sort();
}

function createImportCompletionProvider() {
	return {
		async provideCompletionItems(document, position) {
			const linePrefix = document.lineAt(position.line).text.slice(0, position.character);
			if (!/^\s*use\s+["'][^"']*$/.test(linePrefix)) {
				return [];
			}

			const candidates = await findImportCandidates(document);
			return candidates.map((name) => makeItem(name, vscode.CompletionItemKind.Module, 'Quark import path'));
		},
	};
}

function activate(context) {
	const runDisposable = vscode.commands.registerCommand('quark.runCurrentFile', createRunCommand());
	const generalCompletion = vscode.languages.registerCompletionItemProvider('quark', createGeneralCompletionProvider());
	const methodCompletion = vscode.languages.registerCompletionItemProvider('quark', createMethodCompletionProvider(), '.');
	const importCompletion = vscode.languages.registerCompletionItemProvider('quark', createImportCompletionProvider(), '"', "'", '/');

	context.subscriptions.push(runDisposable, generalCompletion, methodCompletion, importCompletion);
}

function deactivate() {}

module.exports = {
	activate,
	deactivate,
};
