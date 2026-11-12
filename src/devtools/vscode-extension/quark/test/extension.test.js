// Tests for extension.js completion and hover, against a minimal fake of the
// vscode API. Run with: node test/extension.test.js
// (also run by src/core/quark/internal/editordata when node is available).

const assert = require('assert');
const fs = require('fs');
const Module = require('module');
const os = require('os');
const path = require('path');

const captured = {};
const vscode = {
	CompletionItem: class {
		constructor(label, kind) {
			this.label = label;
			this.kind = kind;
		}
	},
	CompletionItemKind: { Keyword: 'keyword', Function: 'function', Module: 'module', Variable: 'variable', Method: 'method', Field: 'field', File: 'file' },
	SnippetString: class {
		constructor(value) {
			this.value = value;
		}
	},
	MarkdownString: class {
		constructor(value = '') {
			this.value = value;
		}
		appendCodeblock(code) {
			this.value += '```\n' + code + '\n```\n';
			return this;
		}
		appendMarkdown(md) {
			this.value += md;
			return this;
		}
	},
	Hover: class {
		constructor(contents, range) {
			this.contents = contents;
			this.range = range;
		}
	},
	workspace: {
		findFiles: async () => [],
		getConfiguration: () => ({ inspect: () => ({}) }),
		workspaceFolders: [],
	},
	languages: {
		registerCompletionItemProvider: (lang, provider) => {
			captured.completion = provider;
			return { dispose() {} };
		},
		registerHoverProvider: (lang, provider) => {
			captured.hover = provider;
			return { dispose() {} };
		},
	},
	commands: { registerCommand: () => ({ dispose() {} }) },
	window: {},
};

const originalLoad = Module._load;
Module._load = function load(request, ...rest) {
	if (request === 'vscode') {
		return vscode;
	}
	return originalLoad.call(this, request, ...rest);
};

const extension = require('../extension.js');
extension.activate({ subscriptions: [] });

// doc builds a fake document; '|' marks the cursor and is removed.
function doc(textWithCursor, fileName = path.join(os.tmpdir(), 'quark-ext-test', 'main.qrk')) {
	const offset = textWithCursor.indexOf('|');
	const text = offset >= 0 ? textWithCursor.slice(0, offset) + textWithCursor.slice(offset + 1) : textWithCursor;
	const lines = text.split('\n');
	const before = text.slice(0, offset).split('\n');
	const position = { line: before.length - 1, character: before[before.length - 1].length };
	const document = {
		fileName,
		uri: { fsPath: fileName },
		isUntitled: false,
		languageId: 'quark',
		lineCount: lines.length,
		lineAt: (i) => ({ text: lines[i] }),
		getText: (range) => {
			if (!range) return text;
			return lines[range.start.line].slice(range.start.character, range.end.character);
		},
		getWordRangeAtPosition: (pos, regex = /[A-Za-z_][A-Za-z0-9_]*/g) => {
			const re = new RegExp(regex.source, 'g');
			const line = lines[pos.line];
			let m;
			while ((m = re.exec(line)) !== null) {
				if (m.index <= pos.character && pos.character <= m.index + m[0].length && m[0].length > 0) {
					return { start: { line: pos.line, character: m.index }, end: { line: pos.line, character: m.index + m[0].length } };
				}
			}
			return undefined;
		},
	};
	return { document, position };
}

async function complete(textWithCursor, triggerCharacter, fileName) {
	const { document, position } = doc(textWithCursor, fileName);
	const items = await captured.completion.provideCompletionItems(document, position, {}, { triggerCharacter });
	return items || [];
}

function hover(textWithCursor) {
	const { document, position } = doc(textWithCursor);
	const h = captured.hover.provideHover(document, position);
	if (!h) return undefined;
	return typeof h.contents === 'string' ? h.contents : h.contents.value;
}

const labels = (items) => items.map((i) => i.label);

const tests = [];
function test(name, fn) {
	tests.push({ name, fn });
}

test('string methods after a str variable', async () => {
	const l = labels(await complete("x = 'hi'\nx.|", '.'));
	assert(l.includes('upper') && l.includes('split'), l.join(','));
	assert(!l.includes('push'), 'list method offered for str');
});

test('list methods after a list literal variable', async () => {
	const l = labels(await complete('y = list [1, 2]\ny.|', '.'));
	assert(l.includes('push') && l.includes('join'), l.join(','));
	assert(!l.includes('upper'));
});

test('method type from a parameter annotation', async () => {
	const l = labels(await complete('fn f(s: str) ->\n    s.|', '.'));
	assert(l.includes('trim') && !l.includes('push'), l.join(','));
});

test('chained call: split returns a list', async () => {
	const l = labels(await complete("parts = 'a,b'.split(',').|", '.'));
	assert(l.includes('join') && !l.includes('upper'), l.join(','));
});

test('method insert text places the cursor inside the parentheses', async () => {
	const items = await complete("x = 'hi'\nx.|", '.');
	const upper = items.find((i) => i.label === 'upper');
	assert.strictEqual(upper.insertText.value, 'upper($0)');
});

test('stdlib module members after an alias', async () => {
	const items = await complete("incl 'std/blas' as blas\nblas::|", ':');
	const dot = items.find((i) => i.label === 'dot');
	assert(dot, labels(items).join(','));
	assert(dot.detail.includes('dot(x: vec, y: vec) float'), dot.detail);
	assert(dot.documentation.value.includes('Dot product'));
});

test('module members from another file', async () => {
	const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'quark-ext-'));
	fs.writeFileSync(path.join(dir, 'shapes.qrk'), 'fn unit() -> 1\nmodule shapes:\n    // Area of a circle.\n    fn area(r: float) float ->\n        3.14 * r * r\nmodule extra:\n    fn more() -> 2\n');
	const main = path.join(dir, 'main.qrk');
	const items = await complete("incl './shapes' as s\ns::|", ':', main);
	const area = items.find((i) => i.label === 'area');
	assert(area && area.detail.includes('area(r: float) float'), labels(items).join(','));
	assert(labels(items).includes('unit'), labels(items).join(','));
	assert(labels(await complete("incl './shapes'\nshapes::extra::|", ':', main)).includes('more'));
	// Modules aren't reached with a dot.
	assert(!labels(await complete("incl './shapes' as s\ns.|", '.', main)).includes('area'));
});

test('same-file module members', async () => {
	const l = labels(await complete('module mathx:\n    fn square(x) -> x * x\nmathx::|', ':'));
	assert(l.includes('square'), l.join(','));
	assert.strictEqual((await complete('x = a :|', ':')).length, 0);
});

test('struct fields', async () => {
	const l = labels(await complete('struct Point:\n    x: float\n    y: float\np = Point { x: 1.0, y: 2.0 }\np.|', '.'));
	assert.deepStrictEqual(l, ['x', 'y']);
});

test('general completion has keywords and builtins with snippet insert text', async () => {
	const items = await complete('pri|');
	const print = items.find((i) => i.label === 'print');
	assert(print && print.insertText.value === 'print($0)');
	assert(items.some((i) => i.label === 'elseif' && i.kind === 'keyword'));
});

test('no completion inside comments or strings', async () => {
	assert.strictEqual((await complete('// x.|', '.')).length, 0);
	assert.strictEqual((await complete("s = 'a.|", '.')).length, 0);
});

test('division slash does not trigger completion', async () => {
	assert.strictEqual((await complete('x = 4 /|', '/')).length, 0);
});

test('import paths list stdlib modules', async () => {
	const l = labels(await complete("incl '|", "'"));
	assert(l.includes('std/io') && l.includes('std/blas'), l.join(','));
});

test('hover explains Python and R habits', () => {
	assert(/fn/.test(hover('d|ef f(x):')));
	assert(/elseif/.test(hover('el|if x:')));
	assert(/true/.test(hover('x = TR|UE')));
	assert(/and/.test(hover('x = a &|& b')));
	assert(/\/\//.test(hover('# a com|ment')));
	assert(/\/\//.test(hover('x = 1  # trailing com|ment')));
	assert.strictEqual(hover("s = '#not a com|ment'"), undefined);
});

test('hover shows builtin and module signatures', () => {
	assert(/len\(any\) int/.test(hover('n = le|n(x)')));
	assert(/Dot product/.test(hover("incl 'std/blas' as blas\nblas::d|ot(x, y)")));
	assert(/blas::dot/.test(hover("incl 'std/blas' as blas\nblas::d|ot(x, y)")));
});

(async () => {
	let failed = 0;
	for (const t of tests) {
		try {
			await t.fn();
			console.log(`ok   ${t.name}`);
		} catch (e) {
			failed += 1;
			console.log(`FAIL ${t.name}\n     ${e.message}`);
		}
	}
	console.log(`${tests.length - failed}/${tests.length} passed`);
	process.exit(failed > 0 ? 1 : 0);
})();
