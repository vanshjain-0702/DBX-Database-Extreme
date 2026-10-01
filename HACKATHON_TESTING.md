# DBX Testing Instructions for Hackathon Judges

Welcome to the DBX testing guide! DBX is a high-performance vector and key-value database designed for AI workloads with strict multi-tenant isolation. This guide will walk you through starting the backend orchestrator, checking the control plane, and running our flagship AI use case (Jurix) which demonstrates DBX's capabilities in real-time.

## Prerequisites

- **Go** (1.22+)
- **Node.js** (v18+) and npm
- **Python** (3.10+)
- **Powershell** (on Windows) or Bash (on Linux/Mac)

---

## Step 1: Start the DBX Engine (Backend)

The DBX Engine is the core database daemon that handles multi-tenant provisioning, Raft consensus (if enabled), and REST/RESP API routing.

1. Open a terminal in the root of the repository.
2. Build the DBX Orchestrator:
   ```bash
   go build -o dbx-orchestrator.exe ./cmd/dbx-orchestrator
   ```
3. Start the orchestrator with the necessary environment variables:
   ```powershell
   # Windows (Powershell)
   $env:DBX_ADMIN_PASSWORD="adminadminadmin"
   $env:DBX_JWT_SECRET="supersecretjwtsecret1234567890123456"
   $env:DBX_INTERNAL_API_TOKEN="internalapitoken1234567890123456"
   $env:DBX_DEFAULT_PASSWORD="adminadminadmin"
   $env:DBX_DATA_DIR="./data"
   $env:DBX_NODE_MEMORY_BUDGET="8gb"
   .\dbx-orchestrator.exe -insecure-http=true
   ```
   *(For Linux/Mac, export the variables and run `./dbx-orchestrator -insecure-http=true`)*

Keep this terminal running. The orchestrator listens on port `8000`.

---

## Step 2: Start the DBX Control Plane Dashboard

The Dashboard provides a UI for managing tenants, viewing cluster health, and monitoring DBX nodes.

1. Open a **second terminal**.
2. Navigate to the `dashboard` directory:
   ```bash
   cd dashboard
   ```
3. Install dependencies and start the dev server:
   ```bash
   npm install
   npm run dev
   ```
4. Open your browser to `http://localhost:5173`.
5. Login with the credentials:
   - **Username:** `admin`
   - **Password:** `adminadminadmin`

You can explore the dashboard to see DBX tenants get created dynamically in Step 3!

---

## Step 3: Run the Flagship App (Jurix)

Jurix is an AI-powered legal audit application powered entirely by DBX. Every time you create a new client in Jurix, it dynamically provisions a completely isolated tenant database in DBX.

1. Open a **third terminal**.
2. Navigate to the root of the repository.
3. Install the Python requirements:
   ```bash
   pip install -r legal-auditor/requirements.txt
   ```
4. Start the Jurix backend using Uvicorn:
   ```powershell
   # Windows (Powershell)
   $env:DBX_ADMIN_PASSWORD="adminadminadmin"
   python -m uvicorn app:app --app-dir jurix --port 8091
   ```
5. Open your browser to `http://localhost:8091/`.
6. You will see the Jurix login page.
   - **Register a new account** (e.g., test@example.com / any password / your firm name).
   - **Create a New Client Matter**: Click the button to create a new client. This makes an API call to DBX to instantly provision a new multi-tenant vector store database in the background!
   - **Ingest Documents**: Paste any document text and hit "Chunk & Embed".
   - **Audit Query**: Search the documents you just ingested. The queries hit DBX's custom `VSEARCH` API.

---

## Step 4: Run the API Tests

To see the technical features of DBX in action (tenant creation, vector embeddings, querying, and our newly fixed string parser), you can run the Python test suite.

1. In a new terminal, navigate to the root of the repository.
2. Run the test script:
   ```bash
   python test_fixes.py
   ```
   *This script provisions a new tenant dynamically, waits for it to be healthy, and executes DBX queries using both legacy Array formats and modern String formats.*

## Step 5: Run Engine Unit Tests

To verify the core engine functionality, storage layer, and isolation guarantees:

```bash
cd internal/engine
go test -v
```

---
### Key Features to Look For:
- **Instant Provisioning**: Notice how fast Jurix can create a completely new tenant database on DBX.
- **Strict Isolation**: Each client in Jurix is assigned a completely isolated vector database at the process level (Linux Landlock) or in-memory (Windows).
- **VSEARCH**: Fast similarity search queries handled directly by the DBX engine.
