const tls = require('tls');
const fs = require('fs');
const path = require('path');
const http = require('http');

// Configs
const RESP_PORT = 6401;
const HTTP_PORT = 8081;
const HOST = '127.0.0.1';
const DIM = 384;

// Load certs
const certsDir = path.join(__dirname, '..', 'certs');
const tlsOptions = {
  ca: fs.readFileSync(path.join(certsDir, 'ca.crt')),
  key: fs.readFileSync(path.join(certsDir, 'client.key')),
  cert: fs.readFileSync(path.join(certsDir, 'client.crt')),
  rejectUnauthorized: false
};

// Generate a random normalized vector
function randomVector(dim) {
  const vec = Array.from({ length: dim }, () => (Math.random() * 2) - 1);
  const norm = Math.sqrt(vec.reduce((sum, v) => sum + v * v, 0));
  return vec.map(v => (v / (norm || 1)).toFixed(5));
}

// Format raw RESP command
function formatCommand(args) {
  let cmd = `*${args.length}\r\n`;
  for (const arg of args) {
    const s = String(arg);
    cmd += `$${Buffer.byteLength(s)}\r\n${s}\r\n`;
  }
  return cmd;
}

// Persistent RESP connection helper
function connectRESP() {
  return new Promise((resolve, reject) => {
    const socket = tls.connect(RESP_PORT, HOST, tlsOptions, () => {
      resolve(socket);
    });
    socket.on('error', reject);
  });
}

// Execute query on RESP socket
function queryRESP(socket, args) {
  return new Promise((resolve, reject) => {
    const cmd = formatCommand(args);
    
    const onData = (data) => {
      socket.removeListener('data', onData);
      socket.removeListener('error', onError);
      resolve(data.toString());
    };
    
    const onError = (err) => {
      socket.removeListener('data', onData);
      socket.removeListener('error', onError);
      reject(err);
    };

    socket.on('data', onData);
    socket.on('error', onError);
    socket.write(cmd);
  });
}

// Execute query via HTTP API
function queryHTTP(args) {
  return new Promise((resolve, reject) => {
    const payload = JSON.stringify({ command: args });
    const req = http.request({
      hostname: HOST,
      port: HTTP_PORT,
      path: '/query',
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Content-Length': Buffer.byteLength(payload)
      }
    }, (res) => {
      let body = '';
      res.on('data', chunk => body += chunk);
      res.on('end', () => resolve(body));
    });
    
    req.on('error', reject);
    req.write(payload);
    req.end();
  });
}

// Sleep helper to simulate network RTT
function delay(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

async function run() {
  console.log("=== DBX REAL-WORLD PERFORMANCE & WAN SIMULATOR ===");
  console.log(`Connecting to RESP port ${RESP_PORT} and HTTP port ${HTTP_PORT}...`);

  const dim = 384;
  console.log("Connected to DBX. Using pre-ingested 'big_web_index' with real world text embeddings...");
  const INDEX_NAME = "big_web_index";

  // Test suite configurations
  const CONCURRENCY = 50;
  const REQUESTS_PER_CLIENT = 10;
  const SIMULATED_LATENCIES = [
    { name: "Intra-Datacenter VPC (1.5ms RTT)", ms: 1.5 },
    { name: "Serverless-to-DB Edge (15ms RTT)", ms: 15.0 },
    { name: "Cross-Region WAN (60ms RTT)", ms: 60.0 }
  ];

  // 1. RESP WORKLOAD BENCHMARK
  console.log("--- WORKLOAD 1: Persistent RESP Connection Pool (Redis Protocol) ---");
  for (const lat of SIMULATED_LATENCIES) {
    console.log(`Running with ${CONCURRENCY} concurrent client connections, ${lat.name}...`);
    const clients = await Promise.all(Array.from({ length: CONCURRENCY }, () => connectRESP()));
    
    const startTime = Date.now();
    const latencies = [];

    const tasks = clients.map(async (socket) => {
      for (let r = 0; r < REQUESTS_PER_CLIENT; r++) {
        const qVec = randomVector(dim);
        const reqStart = Date.now();
        // Simulate sending request + waiting for network trip
        await delay(lat.ms / 2);
        await queryRESP(socket, ["VSEARCH", INDEX_NAME, ...qVec, 5]);
        await delay(lat.ms / 2);
        
        const reqDur = Date.now() - reqStart;
        latencies.push(reqDur);
      }
      socket.destroy();
    });

    await Promise.all(tasks);
    const totalTime = (Date.now() - startTime) / 1000;
    const avgLat = latencies.reduce((a, b) => a + b, 0) / latencies.length;
    const sorted = [...latencies].sort((a, b) => a - b);
    const p95 = sorted[Math.floor(sorted.length * 0.95)];
    const p99 = sorted[Math.floor(sorted.length * 0.99)];
    const qps = (CONCURRENCY * REQUESTS_PER_CLIENT) / totalTime;

    console.log(`  Average Latency: ${avgLat.toFixed(2)} ms`);
    console.log(`  P95 Latency:     ${p95.toFixed(2)} ms`);
    console.log(`  P99 Latency:     ${p99.toFixed(2)} ms`);
    console.log(`  Throughput:      ${qps.toFixed(2)} queries/sec\n`);
  }

  // 2. HTTP WORKLOAD BENCHMARK
  console.log("--- WORKLOAD 2: HTTP JSON API (Connection Overhead & HTTP overhead) ---");
  for (const lat of SIMULATED_LATENCIES) {
    console.log(`Running with ${CONCURRENCY} concurrent client streams, ${lat.name}...`);
    
    const startTime = Date.now();
    const latencies = [];

    const tasks = Array.from({ length: CONCURRENCY }).map(async () => {
      for (let r = 0; r < REQUESTS_PER_CLIENT; r++) {
        const qVec = randomVector(dim);
        const reqStart = Date.now();
        // Simulate sending request + waiting for network trip
        await delay(lat.ms / 2);
        await queryHTTP(["VSEARCH", INDEX_NAME, ...qVec, 5]);
        await delay(lat.ms / 2);
        
        const reqDur = Date.now() - reqStart;
        latencies.push(reqDur);
      }
    });

    await Promise.all(tasks);
    const totalTime = (Date.now() - startTime) / 1000;
    const avgLat = latencies.reduce((a, b) => a + b, 0) / latencies.length;
    const sorted = [...latencies].sort((a, b) => a - b);
    const p95 = sorted[Math.floor(sorted.length * 0.95)];
    const p99 = sorted[Math.floor(sorted.length * 0.99)];
    const qps = (CONCURRENCY * REQUESTS_PER_CLIENT) / totalTime;

    console.log(`  Average Latency: ${avgLat.toFixed(2)} ms`);
    console.log(`  P95 Latency:     ${p95.toFixed(2)} ms`);
    console.log(`  P99 Latency:     ${p99.toFixed(2)} ms`);
    console.log(`  Throughput:      ${qps.toFixed(2)} queries/sec\n`);
  }

  // 3. RAW MAX THROUGHPUT TEST (Local loopback, no simulated delay)
  console.log("--- WORKLOAD 3: Raw Local Throughput (Local Loopback, 100% persistent RESP) ---");
  const maxClients = 50;
  const reqsPerClientMax = 200;
  const maxSockets = await Promise.all(Array.from({ length: maxClients }, () => connectRESP()));
  
  const maxStart = Date.now();
  const maxTasks = maxSockets.map(async (socket) => {
    for (let r = 0; r < reqsPerClientMax; r++) {
      const qVec = randomVector(dim);
      await queryRESP(socket, ["VSEARCH", INDEX_NAME, ...qVec, 5]);
    }
    socket.destroy();
  });

  await Promise.all(maxTasks);
  const maxTotalTime = (Date.now() - maxStart) / 1000;
  const maxOps = maxClients * reqsPerClientMax;
  console.log(`  Executed ${maxOps} queries under max concurrent load.`);
  console.log(`  Total Time:   ${maxTotalTime.toFixed(3)} seconds`);
  console.log(`  Max QPS:      ${(maxOps / maxTotalTime).toFixed(2)} queries/sec`);
  console.log("=================================================");
}

run().catch(console.error);
