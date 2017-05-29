package main

import (
	"golang.org/x/net/html"
	"io"
	"strings"
)

type ParseHTML struct {
	level int
	d     *html.Tokenizer
	Tok   html.Token
}

func (m *ParseHTML) Init(r io.Reader) {
	m.level = 0
	m.d = html.NewTokenizer(r)
}

func (m *ParseHTML) SeekTag(stopStrings ...string) bool {
	for {
		// token type
		tokenType := m.d.Next()
		if tokenType == html.ErrorToken {
			return false
		}
		m.Tok = m.d.Token()
		//		fmt.Println(tokenType, m.Tok)
		switch tokenType {
		case html.StartTagToken: // <tag>
			m.level++
			//			fmt.Printf("%d %s\n", m.level, m.Tok.String())
			//			fmt.Println(m.level, m.Tok)
			if len(m.Tok.Attr) > 0 {
				//				fmt.Printf(" %d %s %s\n", m.level, m.Tok.Attr[0].Key, m.Tok.Attr[0].Val)
				for _, stopString := range stopStrings {
					if strings.Compare(stopString, m.Tok.Attr[0].Val) == 0 {
						//						fmt.Println("StartTagToken stop condition")
						return true
					}
				}
			}
		case html.EndTagToken: // </tag>
			m.level--
			//		case html.TextToken:
			//			fmt.Println(m.level, "text", m.Tok.Data, ":", m.Tok)
		case html.SelfClosingTagToken: // <tag/>
		}
	}
}

func (m *ParseHTML) GetText() string {
	for {
		// token type
		tokenType := m.d.Next()
		if tokenType == html.ErrorToken {
			return "error"
		}
		token := m.d.Token()
		switch tokenType {
		case html.TextToken: // text between start and end tag
			//            fmt.Println(token.String())
			return token.String()
		case html.StartTagToken: // <tag>
			m.level++
		case html.EndTagToken: // </tag>
			m.level--
		case html.SelfClosingTagToken: // <tag/>
		}
	}
}

func (m *ParseHTML) SeekBoldText() (string, bool) {
	boldFound := false
	for {
		tokenType := m.d.Next()
		if tokenType == html.ErrorToken {
			return "nil", false
		}
		m.Tok = m.d.Token()
		switch tokenType {
		case html.StartTagToken: // looking for <b>
			m.level++
			if m.Tok.String() == "<b>" {
				boldFound = true
			}
		case html.EndTagToken: // </tag>
			m.level--
		case html.TextToken:
			if boldFound {
				return m.Tok.Data, true
			}
		case html.SelfClosingTagToken: // <tag/>
		}
	}
}
