package engine

import (
	"bytes"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

var ChapterTitlePattern = regexp.MustCompile(`^(?:第[零一二三四五六七八九十百千万两〇○0-9０-９]+[章回节卷集部]|序章|楔子|引子|前言|尾声|后记|番外(?:篇)?|第[零一二三四五六七八九十百千万两〇○0-9０-９]+卷|[上中下]卷).{0,64}$`)

type TXTChapter struct {
	Index   int    `json:"index"`
	Title   string `json:"title"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	Content string `json:"content"`
}

func ParseTXT(data []byte) ([]TXTChapter, error) {
	text, err := decodeTXT(data)
	if err != nil {
		return nil, err
	}

	chapters := make([]TXTChapter, 0)
	current := TXTChapter{Index: 0, Title: "正文", Start: 0}
	sawChapterTitle := false

	contentStart := 0
	for lineStart := 0; lineStart < len(text); {
		lineEnd := nextLineEnd(text, lineStart)
		lineText := strings.TrimRight(text[lineStart:lineEnd], "\r\n")
		line := strings.TrimSpace(lineText)
		if isChapterTitle(line) {
			if lineStart > contentStart {
				content := strings.TrimSpace(text[contentStart:lineStart])
				if sawChapterTitle || shouldKeepFrontMatter(content) {
					current.Index = len(chapters)
					current.End = lineStart
					current.Content = content
					chapters = append(chapters, current)
				}
			}
			current = TXTChapter{Title: line, Start: lineStart}
			contentStart = lineEnd
			sawChapterTitle = true
		}
		lineStart = lineEnd
	}

	if contentStart <= len(text) {
		content := strings.TrimSpace(text[contentStart:])
		if sawChapterTitle || len(chapters) == 0 || shouldKeepFrontMatter(content) {
			if content != "" || len(chapters) == 0 {
				current.Index = len(chapters)
				current.End = len(text)
				current.Content = content
				chapters = append(chapters, current)
			}
		}
	}

	return chapters, nil
}

func nextLineEnd(text string, start int) int {
	for index := start; index < len(text); index++ {
		if text[index] == '\n' {
			return index + 1
		}
	}
	return len(text)
}

func isChapterTitle(line string) bool {
	line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
	if line == "" || utf8.RuneCountInString(line) > 72 {
		return false
	}
	if strings.ContainsAny(rightmostRune(line), "。！？!?；;") {
		return false
	}
	return ChapterTitlePattern.MatchString(line)
}

func rightmostRune(value string) string {
	for len(value) > 0 {
		r, size := utf8.DecodeLastRuneInString(value)
		if r == utf8.RuneError && size == 0 {
			return ""
		}
		return string(r)
	}
	return ""
}

func shouldKeepFrontMatter(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}

	lines := strings.Split(content, "\n")
	nonEmpty := 0
	totalRunes := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		nonEmpty++
		totalRunes += utf8.RuneCountInString(line)
	}

	return nonEmpty > 8 || totalRunes > 360
}

func decodeTXT(data []byte) (string, error) {
	if utf8.Valid(data) {
		return string(data), nil
	}
	decoded, err := simplifiedchinese.GBK.NewDecoder().Bytes(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
