package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"

	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	probing "github.com/prometheus-community/pro-bing"
)

type IPResult struct {
	IP    string
	Delay time.Duration
}

func main() {
	//取命令行第一个参数作为文件名
	if len(os.Args) < 2 {
		fmt.Println("Usage: pingtool <csv_file> [concurrency]")
		return
	}
	csvFile := os.Args[1]
	fmt.Println("csvFile:", csvFile)
	// 读取CSV文件
	file, err := os.Open(csvFile)
	if err != nil {
		fmt.Println("Failed to open CSV file:", err)
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Println("Failed to read CSV file:", err)
		return
	}

	var ips []string
	for _, record := range records {
		if len(record) > 0 {
			ips = append(ips, record[0])
		}
	}

	concurrency := 300
	if len(os.Args) > 2 {
		concurrency, err = strconv.Atoi(os.Args[2])
		if err != nil {
			fmt.Println("Invalid concurrency value, using default 300")
			concurrency = 300
		}
	}

	myApp := app.New()
	w := myApp.NewWindow("Ping Tool")

	topList := widget.NewLabel("Top 20 Fastest IPs:\n")
	progressLabel := widget.NewLabel("Progress:\n")
	ongoingLabel := widget.NewLabel("Ongoing Ping IPs:\n")

	content := container.NewVBox(topList, progressLabel, ongoingLabel)
	w.SetContent(content)

	var results []IPResult
	var mutex sync.Mutex
	var wg sync.WaitGroup

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ticker.C:
				mutex.Lock()
				sort.Slice(results, func(i, j int) bool {
					return results[i].Delay < results[j].Delay
				})

				top20 := "Top 20 Fastest IPs:\n"
				for i, result := range results {
					if i >= 20 {
						break
					}
					top20 += fmt.Sprintf("%s: %d\n", result.IP, result.Delay.Milliseconds())
				}
				topList.SetText(top20)

				progress := fmt.Sprintf("Progress: %d/%d\n", len(results), len(ips))
				progressLabel.SetText(progress)

				mutex.Unlock()
			}
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("Interrupt received, stopping...")
		os.Exit(0)
	}()

	sem := make(chan struct{}, concurrency)
	for _, ip := range ips {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			pinger, err := probing.NewPinger(ip) // ping.NewPinger(ip)
			if err != nil {
				fmt.Println("Failed to ping IP:", ip, err)
				return
			}

			pinger.Count = 2
			pinger.SetPrivileged(true)
			pinger.Timeout = time.Second * 3
			err = pinger.Run()
			if err != nil {
				fmt.Println("Failed to run ping:", err)
				return
			}
			stats := pinger.Statistics()

			mutex.Lock()
			if stats.AvgRtt == 0 {
				results = append(results, IPResult{IP: ip, Delay: time.Second * 3})
			} else {
				results = append(results, IPResult{IP: ip, Delay: stats.AvgRtt})
			}
			ongoingLabel.SetText(fmt.Sprintf("Ongoing Ping IPs: %s", ip))
			mutex.Unlock()
		}(ip)
	}

	go func() {
		wg.Wait()
		//myApp.Quit()
	}()

	w.ShowAndRun()
}
