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
		chromedp.Sleep(2*time.Second), // Wait for initial redirect to start
	)
	if err != nil {
		return nil, cleanFunc, err
	}

	app.logger.Info("Waiting for redirect after login...")

	loginSuccess := false
	timeout := time.After(20 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var currentURL string

	for !loginSuccess {
		select {
		case <-timeout:
			app.logger.Error("Login failed: timeout waiting for redirect")
			app.logger.Error("Got:", "url", currentURL)

			// Try to get any error messages on the page
			var errorText string
			err := chromedp.Run(loginCtx, chromedp.Text(`body`, &errorText, chromedp.ByQuery))
			if err != nil {
				app.logger.Error("Failed to get page content", "error", err)
				return nil, cleanFunc, errors.New("login failed: did not redirect to expected URL after 20 seconds")
			}
			if len(errorText) > 0 && len(errorText) < 500 {
				app.logger.Error("Page content:", "text", errorText)
			}

			return nil, cleanFunc, errors.New("login failed: did not redirect to expected URL after 20 seconds")
		case <-ticker.C:
			err = chromedp.Run(loginCtx, chromedp.Location(&currentURL))
			if err != nil {
				return nil, cleanFunc, err
			}

			// Login is successful if URL is no longer the login page
			if currentURL != "https://study.migaku.com/login" {
				app.logger.Info("Detected redirect to:", "url", currentURL)
				loginSuccess = true
			}
		}
	}

	app.logger.Info("Login successful", "url", currentURL)

	// Ensure page is fully loaded
	err = chromedp.Run(loginCtx, chromedp.WaitReady("body"))
	if err != nil {
		return nil, cleanFunc, err
	}

	app.isAuthenticated.Store(true)

	app.logger.Info("Browser initialized and ready")
	return loginCtx, cleanFunc, nil
}
