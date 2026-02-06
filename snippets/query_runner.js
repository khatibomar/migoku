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
    const query = __QUERY__;
    const params = __PARAMS__;

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
