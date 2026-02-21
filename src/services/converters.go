package services

import (
	"archive/zip"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/go-pdf/fpdf"
	"github.com/ledongthuc/pdf"
	"github.com/xuri/excelize/v2"
	"golang.org/x/net/html"
)

type Converter interface {
	Convert(inputPath, outputPath string) error
}

var converterRegistry = map[string]map[string]Converter{
	"txt":  {"pdf": &TextToPDFConverter{}},
	"csv":  {"pdf": &CSVToPDFConverter{}, "xlsx": &CSVToExcelConverter{}},
	"md":   {"txt": &StripMarkdownConverter{}},
	"html": {"txt": &HTMLToTextConverter{}},
	"pdf":  {"txt": &PDFToTextConverter{}},
	"docx": {"txt": &DOCXToTextConverter{}},
}

func GetConverter(sourceFormat, targetFormat string) (Converter, bool) {
	targets, ok := converterRegistry[sourceFormat]
	if !ok {
		return nil, false
	}
	conv, ok := targets[targetFormat]
	return conv, ok
}

func GetSupportedConversions() map[string][]string {
	result := make(map[string][]string)
	for src, targets := range converterRegistry {
		for tgt := range targets {
			result[src] = append(result[src], tgt)
		}
	}
	return result
}

// --- TextToPDFConverter ---

type TextToPDFConverter struct{}

func (c *TextToPDFConverter) Convert(inputPath, outputPath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	p := fpdf.New("P", "mm", "A4", "")
	p.SetAutoPageBreak(true, 15)
	p.AddPage()
	p.SetFont("Courier", "", 10)
	p.MultiCell(0, 5, string(data), "", "L", false)

	if p.Err() {
		return fmt.Errorf("fpdf error: %w", p.Error())
	}
	return p.OutputFileAndClose(outputPath)
}

// --- CSVToPDFConverter ---

type CSVToPDFConverter struct{}

func (c *CSVToPDFConverter) Convert(inputPath, outputPath string) error {
	f, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open csv: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("parse csv: %w", err)
	}
	if len(records) == 0 {
		return fmt.Errorf("csv file is empty")
	}

	p := fpdf.New("L", "mm", "A4", "")
	p.SetAutoPageBreak(true, 15)
	p.AddPage()

	numCols := len(records[0])
	pageWidth, _ := p.GetPageSize()
	marginLeft, _, marginRight, _ := p.GetMargins()
	usable := pageWidth - marginLeft - marginRight
	colWidth := usable / float64(numCols)
	if colWidth < 15 {
		colWidth = 15
	}

	// Header row
	p.SetFont("Helvetica", "B", 8)
	for _, cell := range records[0] {
		p.CellFormat(colWidth, 7, truncate(cell, int(colWidth/2)), "1", 0, "C", false, 0, "")
	}
	p.Ln(-1)

	// Data rows
	p.SetFont("Helvetica", "", 7)
	for _, row := range records[1:] {
		for i := 0; i < numCols; i++ {
			val := ""
			if i < len(row) {
				val = row[i]
			}
			p.CellFormat(colWidth, 6, truncate(val, int(colWidth/2)), "1", 0, "L", false, 0, "")
		}
		p.Ln(-1)
	}

	if p.Err() {
		return fmt.Errorf("fpdf error: %w", p.Error())
	}
	return p.OutputFileAndClose(outputPath)
}

func truncate(s string, maxLen int) string {
	if maxLen < 4 {
		maxLen = 4
	}
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}

// --- CSVToExcelConverter ---

type CSVToExcelConverter struct{}

func (c *CSVToExcelConverter) Convert(inputPath, outputPath string) error {
	f, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open csv: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("parse csv: %w", err)
	}

	xlsx := excelize.NewFile()
	defer xlsx.Close()
	sheet := "Sheet1"

	for rowIdx, row := range records {
		for colIdx, cell := range row {
			cellName, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+1)
			xlsx.SetCellValue(sheet, cellName, cell)
		}
	}

	return xlsx.SaveAs(outputPath)
}

// --- StripMarkdownConverter ---

type StripMarkdownConverter struct{}

var mdPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^#{1,6}\s+`),              // headings
	regexp.MustCompile(`\*\*(.+?)\*\*`),            // bold
	regexp.MustCompile(`\*(.+?)\*`),                // italic
	regexp.MustCompile(`__(.+?)__`),                // bold alt
	regexp.MustCompile(`_(.+?)_`),                  // italic alt
	regexp.MustCompile(`~~(.+?)~~`),                // strikethrough
	regexp.MustCompile("`([^`]+)`"),                // inline code
	regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`),    // links
	regexp.MustCompile(`!\[([^\]]*)\]\([^)]+\)`),   // images
	regexp.MustCompile(`^>\s?`),                    // blockquotes
	regexp.MustCompile(`^[-*+]\s+`),                // unordered lists
	regexp.MustCompile(`^\d+\.\s+`),                // ordered lists
	regexp.MustCompile("^```[\\s\\S]*?```"),         // fenced code blocks
	regexp.MustCompile(`^---+$`),                   // horizontal rules
}

func (c *StripMarkdownConverter) Convert(inputPath, outputPath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	text := string(data)

	// Remove fenced code block markers
	text = regexp.MustCompile("(?m)^```[a-zA-Z]*\\s*$").ReplaceAllString(text, "")

	lines := strings.Split(text, "\n")
	var result []string
	for _, line := range lines {
		l := line
		for _, p := range mdPatterns {
			if p.NumSubexp() > 0 {
				l = p.ReplaceAllString(l, "$1")
			} else {
				l = p.ReplaceAllString(l, "")
			}
		}
		result = append(result, l)
	}

	return os.WriteFile(outputPath, []byte(strings.Join(result, "\n")), 0644)
}

// --- HTMLToTextConverter ---

type HTMLToTextConverter struct{}

func (c *HTMLToTextConverter) Convert(inputPath, outputPath string) error {
	f, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open html: %w", err)
	}
	defer f.Close()

	var textBuilder strings.Builder
	tokenizer := html.NewTokenizer(f)

	skipTags := map[string]bool{"script": true, "style": true, "head": true}
	var skipDepth int

	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			if tokenizer.Err() == io.EOF {
				return os.WriteFile(outputPath, []byte(strings.TrimSpace(textBuilder.String())), 0644)
			}
			return fmt.Errorf("html parse error: %w", tokenizer.Err())
		case html.StartTagToken:
			tn, _ := tokenizer.TagName()
			tag := string(tn)
			if skipTags[tag] {
				skipDepth++
			}
			if tag == "br" || tag == "p" || tag == "div" || tag == "li" || tag == "tr" || tag == "h1" || tag == "h2" || tag == "h3" || tag == "h4" || tag == "h5" || tag == "h6" {
				textBuilder.WriteString("\n")
			}
		case html.EndTagToken:
			tn, _ := tokenizer.TagName()
			if skipTags[string(tn)] && skipDepth > 0 {
				skipDepth--
			}
		case html.TextToken:
			if skipDepth == 0 {
				text := strings.TrimSpace(string(tokenizer.Text()))
				if text != "" {
					textBuilder.WriteString(text)
					textBuilder.WriteString(" ")
				}
			}
		}
	}
}

// --- PDFToTextConverter ---

type PDFToTextConverter struct{}

func (c *PDFToTextConverter) Convert(inputPath, outputPath string) error {
	f, reader, err := pdf.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open pdf: %w", err)
	}
	defer f.Close()

	var textBuilder strings.Builder
	numPages := reader.NumPage()

	for i := 1; i <= numPages; i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		textBuilder.WriteString(text)
		if i < numPages {
			textBuilder.WriteString("\n\n")
		}
	}

	return os.WriteFile(outputPath, []byte(textBuilder.String()), 0644)
}

// --- DOCXToTextConverter ---

type DOCXToTextConverter struct{}

type docxBody struct {
	Paragraphs []docxParagraph `xml:"body>p"`
}

type docxParagraph struct {
	Runs []docxRun `xml:"r"`
}

type docxRun struct {
	Text string `xml:"t"`
}

func (c *DOCXToTextConverter) Convert(inputPath, outputPath string) error {
	r, err := zip.OpenReader(inputPath)
	if err != nil {
		return fmt.Errorf("open docx zip: %w", err)
	}
	defer r.Close()

	var docFile *zip.File
	for _, f := range r.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return fmt.Errorf("word/document.xml not found in docx")
	}

	rc, err := docFile.Open()
	if err != nil {
		return fmt.Errorf("open document.xml: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("read document.xml: %w", err)
	}

	var body docxBody
	if err := xml.Unmarshal(data, &body); err != nil {
		return fmt.Errorf("parse document.xml: %w", err)
	}

	var lines []string
	for _, para := range body.Paragraphs {
		var parts []string
		for _, run := range para.Runs {
			if run.Text != "" {
				parts = append(parts, run.Text)
			}
		}
		lines = append(lines, strings.Join(parts, ""))
	}

	return os.WriteFile(outputPath, []byte(strings.Join(lines, "\n")), 0644)
}
