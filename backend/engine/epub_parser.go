package engine

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type ParsedBook struct {
	Title    string
	Author   string
	Chapters []TXTChapter
}

type epubContainer struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

type epubPackage struct {
	Metadata struct {
		Titles   []string `xml:"title"`
		Creators []string `xml:"creator"`
	} `xml:"metadata"`
	Manifest struct {
		Items []epubManifestItem `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		ItemRefs []struct {
			IDRef string `xml:"idref,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

type epubManifestItem struct {
	ID        string `xml:"id,attr"`
	Href      string `xml:"href,attr"`
	MediaType string `xml:"media-type,attr"`
}

func ParseEPUB(data []byte) (ParsedBook, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ParsedBook{}, err
	}

	opfPath, err := epubOPFPath(reader)
	if err != nil {
		return ParsedBook{}, err
	}

	opfBytes, err := readZipFile(reader, opfPath)
	if err != nil {
		return ParsedBook{}, err
	}

	var pkg epubPackage
	if err := xml.Unmarshal(opfBytes, &pkg); err != nil {
		return ParsedBook{}, err
	}

	manifest := make(map[string]epubManifestItem)
	for _, item := range pkg.Manifest.Items {
		manifest[item.ID] = item
	}

	book := ParsedBook{
		Title:  firstNonEmpty(pkg.Metadata.Titles),
		Author: firstNonEmpty(pkg.Metadata.Creators),
	}
	baseDir := path.Dir(opfPath)
	if baseDir == "." {
		baseDir = ""
	}

	for _, ref := range pkg.Spine.ItemRefs {
		item, ok := manifest[ref.IDRef]
		if !ok || !isReadableEPUBItem(item.MediaType) {
			continue
		}

		href, err := url.PathUnescape(item.Href)
		if err != nil {
			href = item.Href
		}
		chapterPath := path.Clean(path.Join(baseDir, href))
		chapterBytes, err := readZipFile(reader, chapterPath)
		if err != nil {
			continue
		}

		title, content := extractEPUBChapter(chapterBytes)
		if strings.TrimSpace(content) == "" {
			continue
		}
		index := len(book.Chapters)
		if title == "" {
			title = fmt.Sprintf("第 %d 章", index+1)
		}
		book.Chapters = append(book.Chapters, TXTChapter{
			Index:   index,
			Title:   title,
			Content: content,
		})
	}

	if len(book.Chapters) == 0 {
		return ParsedBook{}, errors.New("no readable epub chapters found")
	}
	return book, nil
}

func epubOPFPath(reader *zip.Reader) (string, error) {
	data, err := readZipFile(reader, "META-INF/container.xml")
	if err != nil {
		return "", err
	}

	var container epubContainer
	if err := xml.Unmarshal(data, &container); err != nil {
		return "", err
	}
	for _, rootfile := range container.Rootfiles {
		if rootfile.FullPath != "" {
			return rootfile.FullPath, nil
		}
	}
	return "", errors.New("missing opf rootfile")
}

func readZipFile(reader *zip.Reader, name string) ([]byte, error) {
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer opened.Close()
		return io.ReadAll(opened)
	}
	return nil, fmt.Errorf("zip file not found: %s", name)
}

func isReadableEPUBItem(mediaType string) bool {
	mediaType = strings.ToLower(mediaType)
	return mediaType == "application/xhtml+xml" || mediaType == "text/html" || mediaType == ""
}

func extractEPUBChapter(data []byte) (string, string) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return "", ""
	}
	doc.Find("script, style, nav").Remove()

	title := strings.TrimSpace(doc.Find("h1, h2, title").First().Text())
	lines := make([]string, 0)
	doc.Find("h1, h2, h3, p, li, blockquote").Each(func(_ int, selection *goquery.Selection) {
		text := strings.Join(strings.Fields(selection.Text()), " ")
		if text != "" {
			lines = append(lines, text)
		}
	})
	if len(lines) == 0 {
		bodyText := strings.Join(strings.Fields(doc.Find("body").Text()), " ")
		if bodyText != "" {
			lines = append(lines, bodyText)
		}
	}
	return title, strings.Join(lines, "\n")
}

func firstNonEmpty(values []string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
