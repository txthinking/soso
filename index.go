package main

import (
	context "context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/document"
	"github.com/mmcdole/gofeed"
	gse "github.com/vcaesar/gse-bleve"
)

var hc = &http.Client{
	Timeout: 5 * time.Second,
}

var index bleve.Index

func goindex() {
	_, err0 := os.Stat("/yugong.db")
	if err0 != nil {
		if !os.IsNotExist(err0) {
			hi(err0.Error())
			return
		}
		var err error
		opt := gse.Option{
			Index: "/yugong.db",
			Dicts: "embed, zh",
			// Stop:  "",
			// Opt:   "search-hmm",
			// Trim:  "trim",
		}
		index, err = gse.New(opt)
		if err != nil {
			hi(err.Error())
			return
		}
	}
	if err0 == nil {
		var err error
		index, err = bleve.Open("/yugong.db")
		if err != nil {
			hi(err.Error())
			return
		}
	}
	var last, now int64
	for {
		now = time.Now().Unix()
		l := []*Website{}
		if err := db.DB.Select(&l, "select * from Website"); err != nil {
			hi(err.Error())
			return
		}
		for _, v := range l {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			fp := gofeed.NewParser()
			fp.UserAgent = "txthinking.com"
			feed, err := fp.ParseURLWithContext(v.Sitemap, ctx)
			if err != nil {
				hi(map[string]interface{}{
					"When":   "ParseURLWithContext",
					"Target": v.Sitemap,
					"Error":  err,
				})
				continue
			}

			for _, vv := range feed.Items {
				if !strings.HasPrefix(vv.Link, v.URL) || vv.PublishedParsed == nil {
					continue
				}
				d, err := index.Document(vv.Link)
				if err != nil {
					hi(map[string]interface{}{
						"When":   "index.Document",
						"Target": vv.Link,
						"Error":  err,
					})
					continue
				}
				if d == nil {
					t, c, err := fetcharticle(vv.Link, v.Selector)
					if err != nil {
						hi(map[string]interface{}{
							"When":   "fetcharticle",
							"Target": vv.Link,
							"Error":  err,
						})
						continue
					}
					a := Article{
						URL:          vv.Link,
						Title:        t,
						Content:      c,
						UpdatedAt:    vv.PublishedParsed.Unix(),
						ClickedCount: 0,
					}
					if err := index.Index(a.URL, a); err != nil {
						hi(map[string]interface{}{
							"When":   "index.Index",
							"Target": a.URL,
							"Error":  err,
						})
						continue
					}
					continue
				}
				if vv.UpdatedParsed == nil {
					continue
				}
				if vv.UpdatedParsed.Unix() > last {
					a := Article{}
					for _, v := range d.(*document.Document).Fields {
						switch vv := v.(type) {
						case *document.TextField:
							if vv.Name() == "URL" {
								a.URL = string(vv.Value())
							}
							if vv.Name() == "Title" {
								a.Title = string(vv.Value())
							}
							if vv.Name() == "Content" {
								a.Content = string(vv.Value())
							}
						case *document.NumericField:
							if vv.Name() == "UpdatedAt" {
								f, err := vv.Number()
								if err != nil {
									hi(err.Error())
									continue
								}
								a.UpdatedAt = int64(f)
							}
							if vv.Name() == "ClickedCount" {
								f, err := vv.Number()
								if err != nil {
									hi(err.Error())
									continue
								}
								a.ClickedCount = int64(f)
							}
						}
					}
					t, c, err := fetcharticle(vv.Link, v.Selector)
					if err != nil {
						hi(map[string]interface{}{
							"When":   "fetcharticle",
							"Target": vv.Link,
							"Error":  err,
						})
						continue
					}
					a.Title = t
					a.Content = c
					a.UpdatedAt = vv.UpdatedParsed.Unix()
					if err := index.Index(a.URL, a); err != nil {
						hi(map[string]interface{}{
							"When":   "index.Index",
							"Target": a.URL,
							"Error":  err,
						})
						continue
					}
				}
			}
		}
		last = now
		time.Sleep(24 * time.Hour)
	}
}

func fetcharticle(url, selector string) (string, string, error) {
	r, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "", err
	}
	r.Header.Set("User-Agent", "txthinking.com")
	res, err := hc.Do(r)
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", "", errors.New(res.Status)
	}
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return "", "", err
	}
	t := doc.Find("title").Text()
	s := doc.Find(selector)
	s.Find("script").Remove()
	c := s.Text()
	if t == "" || c == "" {
		return "", "", errors.New("No title or article")
	}
	return t, c, nil
}
