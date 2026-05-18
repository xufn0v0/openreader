package engine

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"openreader/backend/models"
)

func TestParseTXTDetectsChineseChapterTitles(t *testing.T) {
	input := []byte("第一章、初见\n这一章的正文。\n第二章再会\n下一章的正文。")

	chapters, err := ParseTXT(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(chapters))
	}
	if chapters[0].Title != "第一章、初见" {
		t.Fatalf("unexpected first title: %q", chapters[0].Title)
	}
	if chapters[1].Title != "第二章再会" {
		t.Fatalf("unexpected second title: %q", chapters[1].Title)
	}
	if chapters[1].Content != "下一章的正文。" {
		t.Fatalf("unexpected second content: %q", chapters[1].Content)
	}
	if chapters[0].Start != 0 || chapters[0].End <= chapters[0].Start {
		t.Fatalf("unexpected first offsets: start=%d end=%d", chapters[0].Start, chapters[0].End)
	}
	if chapters[1].Start < chapters[0].End || chapters[1].End != len(string(input)) {
		t.Fatalf("unexpected second offsets: start=%d end=%d", chapters[1].Start, chapters[1].End)
	}
}

func TestParseTXTSkipsShortFrontMatterBeforeFirstChapter(t *testing.T) {
	input := []byte("测试书名\n作者：某人\n分类：仙侠\n\n序章、剑宗少年\n序章正文。\n第一章、缘起\n第一章正文。\n第四十一章 夺异宝\n第四十一章正文。")

	chapters, err := ParseTXT(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(chapters))
	}
	if chapters[0].Title != "序章、剑宗少年" {
		t.Fatalf("front matter was not skipped, first title: %q", chapters[0].Title)
	}
	if chapters[1].Title != "第一章、缘起" {
		t.Fatalf("unexpected second title: %q", chapters[1].Title)
	}
	if chapters[2].Title != "第四十一章 夺异宝" {
		t.Fatalf("unexpected third title: %q", chapters[2].Title)
	}
}

func TestParseEPUBUsesSpineOrder(t *testing.T) {
	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)
	writeZipFile(t, zipWriter, "META-INF/container.xml", `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`)
	writeZipFile(t, zipWriter, "OEBPS/content.opf", `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>测试 EPUB</dc:title>
    <dc:creator>测试作者</dc:creator>
  </metadata>
  <manifest>
    <item id="chapter2" href="chapter2.xhtml" media-type="application/xhtml+xml"/>
    <item id="chapter1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="chapter1"/>
    <itemref idref="chapter2"/>
  </spine>
</package>`)
	writeZipFile(t, zipWriter, "OEBPS/chapter1.xhtml", `<html><body><h1>第一章</h1><p>第一章正文。</p></body></html>`)
	writeZipFile(t, zipWriter, "OEBPS/chapter2.xhtml", `<html><body><h1>第二章</h1><p>第二章正文。</p></body></html>`)
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	book, err := ParseEPUB(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if book.Title != "测试 EPUB" || book.Author != "测试作者" {
		t.Fatalf("unexpected metadata: %#v", book)
	}
	if len(book.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(book.Chapters))
	}
	if book.Chapters[0].Title != "第一章" || book.Chapters[1].Title != "第二章" {
		t.Fatalf("chapters not in spine order: %#v", book.Chapters)
	}
}

func TestApplyTextReplacementsSupportsRegex(t *testing.T) {
	got := ApplyTextReplacements("广告一\n正文\n广告二", []models.TextReplaceRule{
		{Pattern: `广告.`, Replacement: ""},
	})
	if got != "\n正文\n" {
		t.Fatalf("unexpected replacement result: %q", got)
	}
}

func TestParseSearchResultsIncludesLatestChapter(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<div class="book">
			<a class="name" href="/book/1">测试书</a>
			<span class="author">作者</span>
			<span class="latest">第一百章 新消息</span>
		</div>
	`))
	if err != nil {
		t.Fatal(err)
	}
	results := parseSearchResults(doc, models.BookSourceRule{
		BookListRule:      ".book",
		BookNameRule:      ".name",
		BookAuthorRule:    ".author",
		LatestChapterRule: ".latest",
		BookURLRule:       ".name|attr:href",
	}, models.BookSource{ID: 1, Name: "测试源", BaseURL: "https://source.example"})
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].LatestChapter != "第一百章 新消息" {
		t.Fatalf("latest chapter was not parsed: %+v", results[0])
	}
}

func writeZipFile(t *testing.T, writer *zip.Writer, name string, content string) {
	t.Helper()
	file, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
}
