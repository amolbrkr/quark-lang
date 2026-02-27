const vscode = require('vscode');

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

function activate(context) {
	const runDisposable = vscode.commands.registerCommand('quark.runCurrentFile', createRunCommand());
	context.subscriptions.push(runDisposable);
}

function deactivate() {}

module.exports = {
	activate,
	deactivate,
};
