package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gocolly/colly/v2"
)

func main() {

	url := "https://www.gif-vif.com/gifs/you-are-a-wizard-doggo"
	var gifURL string

	c := colly.NewCollector()

	// Setting up a callback for HTML element tr in tbody
	c.OnHTML(`meta[property="og:image"]`, func(e *colly.HTMLElement) {

		gifURL = e.Attr("content")
		fmt.Println(gifURL)
	})

	// Setting up an error callback
	c.OnError(func(r *colly.Response, err error) {
		log.Printf("Error: %s: Request URL: %s", err, r.Request.URL)
	})

	// Visiting the specified URL to fetch holidays data
	err := c.Visit(url)
	if err != nil {
		log.Fatal(err)
	}

	if gifURL == "" {
		log.Fatal("[ERROR] GIF URL not found")
	}
	// printTable(allHolidays)

	resp, err := http.Get(gifURL)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	out, err := os.Create("wizard_doggo.gif")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Gif saved")
}
