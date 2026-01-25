package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

type languageSelectionResult struct {
	Clicked bool   `json:"clicked"`
	Method  string `json:"method,omitempty"`
}

//go:embed snippets/language_select.js
var languageSelectionScript string

func (app *Application) initializeBrowser(email, password, language string) (context.Context, func(), error) {
	app.isAuthenticated.Store(false)
	userDataDir := filepath.Join(os.TempDir(), "chromedp-user-data")
	if err := os.MkdirAll(userDataDir, os.ModePerm); err != nil {
		app.logger.Error("failed to get temp user data dir", "error", err)
		return nil, nil, err
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", app.headless),
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		chromedp.UserDataDir(userDataDir),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	app.logger.Info("Launching browser...")
	app.logger.Info("Logging in to Migaku...")

	loginCtx, loginCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(app.logger.Debug))

	cleanFunc := func() {
		loginCancel()
		allocCancel()
	}

	err := chromedp.Run(loginCtx,
		chromedp.Navigate("https://study.migaku.com/login"),
		chromedp.WaitVisible("body", chromedp.ByQuery),
	)
	if err != nil {
		return nil, cleanFunc, err
	}

	// Wait a bit for any redirects to complete
	time.Sleep(1 * time.Second)

	var currentURL string
	err = chromedp.Run(loginCtx, chromedp.Location(&currentURL))
	if err != nil {
		return nil, cleanFunc, err
	}

	app.logger.Info("Current URL: " + currentURL)

	// Check if we're still on the login page (not redirected)
	if strings.Contains(currentURL, "/login") {
		app.logger.Info("On login page, checking if login form exists...")

		// Check if login form exists
		var loginFormExists bool
		err = chromedp.Run(loginCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				defer cancel()
				err := chromedp.WaitVisible(`input[type="email"]`, chromedp.ByQuery).Do(timeoutCtx)
				loginFormExists = (err == nil)
				return nil // Don't propagate error
			}),
		)
		if err != nil {
			app.logger.Error("Error checking for login form", "error", err)
			return nil, cleanFunc, err
		}

		if loginFormExists {
			app.logger.Info("Login form found, filling in credentials...")
			err = chromedp.Run(loginCtx,
				chromedp.SendKeys(`input[type="email"]`, email, chromedp.ByQuery),
				chromedp.SendKeys(`input[type="password"]`, password, chromedp.ByQuery),
				chromedp.Sleep(100*time.Millisecond),
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
			// Wait for login form to disappear OR URL to change
			err = chromedp.Run(loginCtx,
				chromedp.ActionFunc(func(ctx context.Context) error {
					timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
					defer cancel()

					// Wait for either form to disappear or URL to change
					ticker := time.NewTicker(200 * time.Millisecond)
					defer ticker.Stop()

					for {
						select {
						case <-timeoutCtx.Done():
							return timeoutCtx.Err()
						case <-ticker.C:
							var newURL string
							if err := chromedp.Location(&newURL).Do(ctx); err == nil {
								if !strings.Contains(newURL, "/login") {
									return nil // Successfully logged in
								}
							}
						}
					}
				}),
			)
			if err != nil {
				app.logger.Info("Login form still present, assuming login failed - proceeding anyway")
			} else {
				app.logger.Info("Login successful")
			}
		} else {
			app.logger.Info("Login form not found, but still on /login URL - likely already logged in, waiting for redirect...")
			// Wait a bit more for redirect
			time.Sleep(2 * time.Second)
			err = chromedp.Run(loginCtx, chromedp.Location(&currentURL))
			if err == nil {
				app.logger.Info("URL after wait: " + currentURL)
			}
		}
	} else {
		app.logger.Info("Already logged in (redirected away from /login)")
	}

	err = chromedp.Run(loginCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			return chromedp.WaitReady("body").Do(ctx)
		}),
	)
	if err != nil {
		app.logger.Warn("Page readiness check failed, but continuing", "error", err)
	}

	if err := app.handleLanguageSelection(loginCtx, language); err != nil {
		return nil, cleanFunc, err
	}

	app.isAuthenticated.Store(true)
	app.logger.Info("Browser initialized and ready")
	return loginCtx, cleanFunc, nil
}

func (app *Application) handleLanguageSelection(ctx context.Context, language string) error {
	var currentURL string
	if err := chromedp.Run(ctx, chromedp.Location(&currentURL)); err != nil {
		return err
	}

	if !strings.Contains(currentURL, "selectLanguage=true") {
		return nil
	}

	normalized := strings.ToLower(strings.TrimSpace(language))
	if normalized == "" {
		return fmt.Errorf("language selection required: set TARGET_LANG env var to a Migaku language code or name")
	}

	app.logger.Info("Language selection required", "language", normalized)

	if err := chromedp.Run(ctx, chromedp.WaitVisible(".LanguageSelect__option", chromedp.ByQuery)); err != nil {
		return err
	}

	langJSON, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("failed to marshal language selection: %w", err)
	}

	script := strings.Replace(languageSelectionScript, "__LANG__", string(langJSON), 1)

	var result languageSelectionResult
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &result)); err != nil {
		return err
	}

	if !result.Clicked {
		return fmt.Errorf("language option not found for %q", normalized)
	}

	app.logger.Info("Language selected", "language", normalized, "method", result.Method)
	return nil
}
