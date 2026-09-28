package lsp

const (
	CompletionItemKindFunction  = 3
	CompletionItemKindVariable  = 6
	CompletionItemKindReference = 18
)

const (
	InsertTextFormatPlainText = 1
	InsertTextFormatSnippet   = 2
)

type CompletionParams struct {
	TextDocumentPositionParams
}

type CompletionRequest struct {
	Request
	Params CompletionParams `json:"params"`
}

type CompletionTextEdit struct {
	Range   TextDocumentPositionRange `json:"range"`
	NewText string                    `json:"newText"`
}

type CompletionItem struct {
	Label            string             `json:"label"`
	Kind             int                `json:"kind"`
	Detail           string             `json:"detail,omitempty"`
	Documentation    string             `json:"documentation,omitempty"`
	SortText         string             `json:"sortText,omitempty"`
	FilterText       string             `json:"filterText,omitempty"`
	InsertText       string             `json:"insertText,omitempty"`
	InsertTextFormat int                `json:"insertTextFormat,omitempty"`
	TextEdit         CompletionTextEdit `json:"textEdit"`
}

type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

type CompletionResponse struct {
	Response
	Result CompletionList `json:"result"`
}
