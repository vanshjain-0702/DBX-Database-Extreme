
const fs = require('fs');
const cssService = require('vscode-css-languageservice').getCSSLanguageService();
const TextDocument = require('vscode-languageserver-textdocument').TextDocument;

['app.css', 'studio.css', 'lexora.css'].forEach(file => {
  const content = fs.readFileSync(file, 'utf8');
  const document = TextDocument.create('file://' + file, 'css', 1, content);
  const stylesheet = cssService.parseStylesheet(document);
  const diagnostics = cssService.doValidation(document, stylesheet);
  console.log('Errors in ' + file + ':', diagnostics.length);
  diagnostics.forEach(d => console.log(' -', d.message));
});

