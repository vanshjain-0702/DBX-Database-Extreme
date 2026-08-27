$ErrorActionPreference = 'Stop'
$base = 'http://[::1]:8001'
$token = (Get-Content -Raw 'data/dashboard-demo/token.json' | ConvertFrom-Json).token
$auth = "Authorization: Bearer $token"

function PostJson([string]$url, $obj) {
  $tmp = Join-Path $env:TEMP ("dbx-demo-" + [guid]::NewGuid().ToString() + ".json")
  $json = $obj | ConvertTo-Json -Compress -Depth 20
  [System.IO.File]::WriteAllText($tmp, $json)
  try {
    curl.exe -sS -X POST $url -H $auth -H "Content-Type: application/json" --data-binary "@$tmp"
  } finally {
    Remove-Item $tmp -Force -ErrorAction SilentlyContinue
  }
}

function Provision([string]$id, [string]$name) {
  Write-Output "provision $id"
  PostJson "$base/api/provision" @{ id = $id; name = $name }
  Write-Output ""
}

function Query([string]$tenant, [string[]]$command) {
  PostJson "$base/t/$tenant/query" @{ command = $command }
}

function UnitVector([int]$dim, [int]$seed) {
  $rnd = New-Object System.Random($seed)
  $vals = New-Object 'float[]' $dim
  $norm = 0.0
  for ($i = 0; $i -lt $dim; $i++) {
    $v = ($rnd.NextDouble() * 2.0) - 1.0
    $vals[$i] = [float]$v
    $norm += $v * $v
  }
  $scale = 1.0 / [math]::Sqrt($norm)
  $out = New-Object 'string[]' $dim
  for ($i = 0; $i -lt $dim; $i++) {
    $out[$i] = ('{0:N6}' -f ($vals[$i] * $scale))
  }
  return $out
}

Provision 'harbor-support' 'Harbor Support Copilot'
Provision 'atlas-legal' 'Atlas Legal RAG'
Provision 'lumen-agents' 'Lumen Agent Platform'
Start-Sleep -Seconds 3

$kv = @(
  @('SET', 'session:acme:u-1842', '{"user":"maya.chen","channel":"chat","intent":"billing","open_ticket":"T-1042"}'),
  @('SET', 'session:acme:u-1901', '{"user":"jon.okonkwo","channel":"email","intent":"mfa_reset","open_ticket":"T-1108"}'),
  @('SET', 'session:acme:u-2014', '{"user":"priya.nair","channel":"chat","intent":"invoice_copy","open_ticket":"T-1120"}'),
  @('SET', 'agent:scratch:u-1842', 'Customer was charged twice on March invoice. Checking Stripe event evt_3N.'),
  @('SET', 'agent:scratch:u-1901', 'MFA reset blocked until last-known device challenge succeeds.'),
  @('SET', 'ticket:T-1042', '{"status":"open","priority":"high","assignee":"copilot"}'),
  @('SET', 'ticket:T-1108', '{"status":"pending","priority":"medium","assignee":"copilot"}'),
  @('SET', 'ticket:T-1120', '{"status":"open","priority":"low","assignee":"human"}'),
  @('SET', 'org:acme:profile', '{"plan":"scale","region":"us-east-1","retention_days":30}'),
  @('SET', 'prompt:support:system', 'You are Acme support memory. Never leak another customer. Cite ticket IDs.'),
  @('INCR', 'metrics:tickets:open'),
  @('INCR', 'metrics:tickets:open'),
  @('INCR', 'metrics:tickets:open'),
  @('EXPIRE', 'session:acme:u-2014', '3600')
)
foreach ($cmd in $kv) {
  Query 'lumen-agents' $cmd | Out-Null
}

$docs = @(
  @{ id = 'ticket-1042'; text = 'Maya Chen was billed twice for the March invoice. Refund the duplicate Stripe charge and email confirmation.' },
  @{ id = 'ticket-1108'; text = 'Jon Okonkwo cannot complete MFA reset. Last known device is a Pixel 8. Require device challenge before reset.' },
  @{ id = 'ticket-1120'; text = 'Priya Nair needs a PDF copy of invoice INV-8841 for accounting. Send to finance@acme.example.' },
  @{ id = 'policy-refunds'; text = 'Refunds under $500 can be issued by the copilot. Larger refunds require a human admin approval.' },
  @{ id = 'policy-mfa'; text = 'Never reset MFA without a second factor from a previously trusted device or a support PIN.' },
  @{ id = 'runbook-billing'; text = 'For duplicate charges: locate the Stripe event, void the second invoice, post credit note, notify the customer.' },
  @{ id = 'faq-hours'; text = 'Acme support hours are 24/5. Weekend coverage is on-call for severity 1 outages only.' },
  @{ id = 'faq-sla'; text = 'Scale plan SLA is 15 minute first response for severity 1, 4 hours for severity 2.' }
)
$dim = 384
foreach ($doc in $docs) {
  $seed = [int]($doc.id.GetHashCode() -band 0x7fffffff)
  $vec = UnitVector $dim $seed
  $add = @('VADD', 'big_web_index', $doc.id) + $vec
  Query 'lumen-agents' $add | Out-Null
  Query 'lumen-agents' @('SET', "doc:big_web_index:$($doc.id)", $doc.text) | Out-Null
}

Query 'atlas-legal' @('SET', 'matter:nw-441', '{"client":"Atlas Legal","topic":"vendor MSA","status":"review"}') | Out-Null
Query 'atlas-legal' @('SET', 'clause:confidentiality', 'Receiving party shall not disclose Confidential Information for 3 years after termination.') | Out-Null
Query 'harbor-support' @('SET', 'agent:router:config', '{"max_steps":12,"memory":"per-customer","model":"lumen-4"}') | Out-Null

Write-Output 'seeded'
curl.exe -sS "$base/api/tenants" -H $auth -o data/dashboard-demo/tenants-list.json
Write-Output 'tenant_count_file_written'
