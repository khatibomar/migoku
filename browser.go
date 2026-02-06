package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

type browserCtx context.Context

type Browser struct {
	ctx      browserCtx
	logger   *slog.Logger
	headless bool
	cleanUp  func()
	language string
	key      string
}

func browserRunContext(ctx context.Context, browser *Browser) (context.Context, context.CancelFunc) {
	runCtx, cancel := context.WithCancel(browser.ctx)
	if ctx == nil {
		return runCtx, cancel
	}

	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-runCtx.Done():
		}
	}()

	return runCtx, cancel
}

func NewBrowser(
	ctx context.Context,
	logger *slog.Logger,
	email, password, language string,
	headless bool,
) (b *Browser, err error) {
	defer func() {
		if err != nil && b != nil && b.cleanUp != nil {
			b.cleanUp()
		}
	}()

	b = &Browser{
		logger:   logger,
		headless: headless,
		language: language,
		key:      email + "|" + language,
	}

	userDataDir := filepath.Join(os.TempDir(), "migoku-browser")
	if err := os.MkdirAll(userDataDir, os.ModePerm); err != nil {
		b.logger.Error("failed to get temp user data dir", "error", err)
		return b, err
	}

	profileDir := "Profile-" + strings.NewReplacer("@", "_", ".", "_", "+", "_").Replace(strings.ToLower(email))
	const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) " +
		"Chrome/120.0.0.0 Safari/537.36"

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", b.headless),
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("profile-directory", profileDir),
		chromedp.UserAgent(userAgent),
		chromedp.UserDataDir(userDataDir),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	b.logger.Info("Launching browser...")
	b.logger.Info("Logging in to Migaku...")

	browserCtx, loginCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(b.logger.Debug))
	b.cleanUp = func() {
		loginCancel()
		allocCancel()
	}
	b.ctx = browserCtx

	runCtx, runCancel := browserRunContext(ctx, b)
	defer runCancel()

	err = chromedp.Run(runCtx,
		chromedp.Navigate("https://study.migaku.com/login"),
		chromedp.WaitVisible("body", chromedp.ByQuery),
	)
	if err != nil {
		return b, err
	}

	// Wait a bit for any redirects to complete
	time.Sleep(1 * time.Second)

	var currentURL string
	err = chromedp.Run(runCtx, chromedp.Location(&currentURL))
	if err != nil {
		return b, err
	}

	b.logger.Info("Current URL: " + currentURL)

	// Check if we're still on the login page (not redirected)
	if strings.Contains(currentURL, "/login") {
		b.logger.Info("On login page, checking if login form exists...")

		// Check if login form exists
		var loginFormExists bool
		err = chromedp.Run(runCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				defer cancel()
				waitErr := chromedp.WaitVisible(`input[type="email"]`, chromedp.ByQuery).Do(timeoutCtx)
				loginFormExists = (waitErr == nil)
				return nil // Don't propagate error
			}),
		)
		if err != nil {
			b.logger.Error("Error checking for login form", "error", err)
			return b, err
		}

		if loginFormExists {
			b.logger.Info("Login form found, filling in credentials...")
			err = chromedp.Run(runCtx,
				chromedp.SendKeys(`input[type="email"]`, email, chromedp.ByQuery),
				chromedp.SendKeys(`input[type="password"]`, password, chromedp.ByQuery),
				chromedp.Sleep(100*time.Millisecond),
			)
			if err != nil {
				return b, err
			}

			b.logger.Info("Submitting login form...")
			err = chromedp.Run(runCtx,
				chromedp.Click(`button[type="submit"]`, chromedp.ByQuery),
			)
			if err != nil {
				return b, err
			}

			b.logger.Info("Waiting for login to complete...")
			// Wait for login form to disappear OR URL to change
			err = chromedp.Run(runCtx,
				chromedp.ActionFunc(func(ctx context.Context) error {
					// Wait for either form to disappear or URL to change
					ticker := time.NewTicker(200 * time.Millisecond)
					defer ticker.Stop()

					for {
						select {
						case <-ctx.Done():
							return ctx.Err()
						case <-ticker.C:
							var newURL string
							if err := chromedp.Location(&newURL).Do(ctx); err == nil {
								if !strings.Contains(newURL, "/login") {
									return nil // Successfully logged in
								}

								var loginFailed bool
								evalErr := chromedp.Evaluate(`(() => {
const email = document.querySelector('input[type="email"][name="email"], input#email');
const password = document.querySelector('input[type="password"][name="password"], input#password');
const hasError = (el) => !!el && (el.classList.contains('-error') || el.classList.contains('error'));
return hasError(email) || hasError(password);
})()`, &loginFailed).Do(ctx)
								if evalErr == nil && loginFailed {
									return errors.New("login failed: invalid credentials")
								}
							}
						}
					}
				}),
			)
			if err != nil {
				err = fmt.Errorf("login process failed: %w", err)
				return b, err
			}

			b.logger.Info("Login successful")
		} else {
			b.logger.Info("Login form not found, but still on /login URL - likely already logged in, waiting for redirect...")
			// Wait a bit more for redirect
			time.Sleep(2 * time.Second)
			err = chromedp.Run(runCtx, chromedp.Location(&currentURL))
			if err == nil {
				b.logger.Info("URL after wait: " + currentURL)
			}
		}
	} else {
		b.logger.Info("Already logged in (redirected away from /login)")
	}

	err = chromedp.Run(runCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			return chromedp.WaitReady("body").Do(ctx)
		}),
	)
	if err != nil {
		b.logger.Warn("Page readiness check failed, but continuing", "error", err)
	}

	if err = b.handleLanguageSelection(runCtx); err != nil {
		return b, err
	}

	b.logger.Info("Browser initialized and ready")
	return b, nil
}

func (b *Browser) handleLanguageSelection(ctx context.Context) error {
	var currentURL string
	if err := chromedp.Run(ctx, chromedp.Location(&currentURL)); err != nil {
		return err
	}

	if !strings.Contains(currentURL, "selectLanguage=true") {
		return nil
	}

	normalized := strings.ToLower(strings.TrimSpace(b.language))
	if normalized == "" {
		return errors.New("language selection required: set TARGET_LANG env var to a Migaku language code or name")
	}

	b.logger.Info("Language selection required", "language", normalized)

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

	b.logger.Info("Language selected", "language", normalized, "method", result.Method)
	return nil
}

func (b *Browser) Close() {
	if b.cleanUp != nil {
		b.cleanUp()
	}
}
