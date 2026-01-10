package main

import (
	"context"
	"errors"
	"time"

	"github.com/chromedp/chromedp"
)

func (app *Application) initializeBrowser(email, password string) (context.Context, func(), error) {
	app.isAuthenticated.Store(false)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", app.headless),
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

	app.logger.Info("Launching browser...")
	app.logger.Info("Logging in to Migaku...")

	loginCtx, loginCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(app.logger.Debug))

	cleanFunc := func() {
		loginCancel()
		allocCancel()
	}

	app.logger.Info("Navigating to login page...")
	err := chromedp.Run(loginCtx,
		chromedp.Navigate("https://study.migaku.com/login"),
		chromedp.WaitVisible(`input[type="email"]`, chromedp.ByQuery),
	)
	if err != nil {
		return nil, cleanFunc, err
	}

	app.logger.Info("Filling in credentials...")
	err = chromedp.Run(loginCtx,
		chromedp.SendKeys(`input[type="email"]`, email, chromedp.ByQuery),
		chromedp.SendKeys(`input[type="password"]`, password, chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond), // Give the form time to validate
	)
	if err != nil {
		return nil, cleanFunc, err
	}

	app.logger.Info("Submitting login form...")
	err = chromedp.Run(loginCtx,
		chromedp.Click(`button[type="submit"]`, chromedp.ByQuery),
	)
	if err != nil {
		return nil, cleanFunc, err
	}

	app.logger.Info("Waiting for login to complete...")

	// Custom wait for URL change - chromedp's navigation detection
	loginSuccess := false
	startTime := time.Now()
	timeout := 30 * time.Second

	var currentURL string
	for !loginSuccess && time.Since(startTime) < timeout {
		err = chromedp.Run(loginCtx, chromedp.Location(&currentURL))
		if err != nil {
			app.logger.Error("Failed to get URL", "error", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		app.logger.Debug("Checking URL", "url", currentURL)

		// Success: URL changed from login page
		if currentURL != "https://study.migaku.com/login" && currentURL != "" {
			app.logger.Info("Navigation detected", "url", currentURL)

			// Wait for new page to start loading
			time.Sleep(500 * time.Millisecond)

			// Verify page is responsive
			var readyState string
			err = chromedp.Run(loginCtx,
				chromedp.Evaluate(`document.readyState`, &readyState),
			)

			if err == nil && (readyState == "interactive" || readyState == "complete") {
				loginSuccess = true
				break
			}

			app.logger.Debug("Page still loading", "readyState", readyState)
		}

		time.Sleep(500 * time.Millisecond)
	}

	if !loginSuccess {
		// Navigation didn't happen - login likely failed
		var pageText string
		if err := chromedp.Run(loginCtx, chromedp.Text(`body`, &pageText, chromedp.ByQuery)); err != nil {
			app.logger.Error("Failed to get page text", "error", err)
		}
		if err := chromedp.Run(loginCtx, chromedp.Location(&currentURL)); err != nil {
			app.logger.Error("Failed to get current URL", "error", err)
		}

		app.logger.Error("Login failed - no navigation occurred")
		app.logger.Error("Current URL", "url", currentURL)
		if len(pageText) > 0 && len(pageText) < 500 {
			app.logger.Error("Page content", "text", pageText)
		}

		return nil, cleanFunc, errors.New("login failed: credentials may be incorrect or page did not redirect")
	}

	app.logger.Info("Login successful", "url", currentURL)

	// Ensure new page is fully loaded
	err = chromedp.Run(loginCtx, chromedp.WaitReady("body"))
	if err != nil {
		return nil, cleanFunc, err
	}

	app.isAuthenticated.Store(true)

	app.logger.Info("Browser initialized and ready")
	return loginCtx, cleanFunc, nil
}
