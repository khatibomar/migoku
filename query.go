package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

func runQuery[T any](app *Application, query string, params ...any) ([]T, error) {
	isAuth := app.isAuthenticated.Load()

	if !isAuth {
		return nil, errors.New("browser not authenticated")
	}

	app.logger.Info("Running query", "query", query, "params", params)

	paramsJSON := "[]"
	if len(params) > 0 {
		paramsBytes, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		paramsJSON = string(paramsBytes)
	}

	script := fmt.Sprintf(`
(async function() {
    const DB_CONFIG = {
        DB_NAME: "srs",
        OBJECT_STORE: "data",
        SQL_CDN_PATH: "https://cdnjs.cloudflare.com/ajax/libs/sql.js/1.10.3/",
    };

    // Load dependencies
    const loadScript = (src) => new Promise((resolve, reject) => {
        if (document.querySelector('script[src="' + src + '"]')) {
            resolve();
            return;
        }
        const script = document.createElement('script');
        script.src = src;
        script.onload = resolve;
        script.onerror = reject;
        document.head.appendChild(script);
    });

    // Load pako (gzip decompression)
    if (!window.pako) {
        await loadScript('https://cdnjs.cloudflare.com/ajax/libs/pako/2.1.0/pako.min.js');
    }

    // Load sql.js
    if (!window.initSqlJs) {
        await loadScript('https://cdnjs.cloudflare.com/ajax/libs/sql.js/1.10.3/sql-wasm.js');
    }

    // Get SQLite blob from IndexedDB
    const getMigakuSQLiteBlob = () => new Promise((resolve, reject) => {
        const request = indexedDB.open(DB_CONFIG.DB_NAME);

        request.onerror = (event) => reject(event.target.error);

        request.onsuccess = (event) => {
            const db = event.target.result;
            if (!db.objectStoreNames.contains(DB_CONFIG.OBJECT_STORE)) {
                reject('Object store not found');
                return;
            }

            const tx = db.transaction([DB_CONFIG.OBJECT_STORE], 'readonly');
            const store = tx.objectStore(DB_CONFIG.OBJECT_STORE);
            const getAllReq = store.getAll();

            getAllReq.onsuccess = () => {
                const records = getAllReq.result;
                if (!records.length || !records[0].data) {
                    reject('No SQLite data found');
                    return;
                }
                resolve(records[0].data);
            };

            getAllReq.onerror = (err) => reject(err.target.error);
        };
    });

    // Main execution
    const blob = await getMigakuSQLiteBlob();
    const decompressed = pako.inflate(blob);

    const SQL = await initSqlJs({
        locateFile: (file) => DB_CONFIG.SQL_CDN_PATH + file,
    });

    const db = new SQL.Database(decompressed);

    // Run the query with parameters
    const query = %s;
    const params = %s;

    let result;
    if (params.length > 0) {
        const stmt = db.prepare(query);
        stmt.bind(params);

        const columns = stmt.getColumnNames();
        const jsonResult = [];

        while (stmt.step()) {
            const row = stmt.get();
            let obj = {};
            columns.forEach((col, index) => {
                obj[col] = row[index];
            });
            jsonResult.push(obj);
        }
        stmt.free();
        result = jsonResult;
    } else {
        const execResult = db.exec(query);

        if (!execResult.length) {
            db.close();
            return [];
        }

        const rows = execResult[0].values;
        const columns = execResult[0].columns;

        result = rows.map(row => {
            let obj = {};
            columns.forEach((col, index) => {
                obj[col] = row[index];
            });
            return obj;
        });
    }

    db.close();
    return result;
})()
`, "`"+query+"`", paramsJSON)

	awaitPromise := func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
		return p.WithAwaitPromise(true)
	}

	var result []T
	eval := chromedp.Evaluate(script, &result, awaitPromise)
	if err := chromedp.Run(app.browserCtx, eval); err != nil {
		app.logger.Error("Query execution failed", "error", err)
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	app.logger.Info("Query completed", "rows", len(result))
	return result, nil
}
