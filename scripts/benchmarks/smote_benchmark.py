# /// script
# requires-python = ">=3.10"
# dependencies = [
#     "pandas",
#     "imbalanced-learn",
#     "redis",
#     "numpy",
# ]
# ///

import pandas as pd
from imblearn.over_sampling import SMOTE
import redis
import time
import numpy as np
import io
import urllib.request

print("DBX SMOTE Benchmarking Pipeline")
print("------------------------------------------")
print("Fetching initial dataset from GitHub...")
url = "https://raw.githubusercontent.com/datasciencedojo/datasets/master/titanic.csv"
req = urllib.request.Request(url, headers={'User-Agent': 'Mozilla/5.0'})
response = urllib.request.urlopen(req)
csv_data = response.read().decode('utf-8')
df = pd.read_csv(io.StringIO(csv_data))

# Select numeric features for vector generation
df = df.dropna(subset=['Age', 'Fare', 'Survived'])
X = df[['Age', 'Fare', 'Pclass', 'SibSp', 'Parch']].values
y = df['Survived'].values

print(f"Original Data: {len(X)} rows")

# Apply SMOTE to generate synthetic minority data
sm = SMOTE(random_state=42)
X_res, y_res = sm.fit_resample(X, y)
print(f"After SMOTE Balancing: {len(X_res)} rows")

# Duplicate data to create a high-volume load test
target_rows = 50000
multiplier = (target_rows // len(X_res)) + 1
X_res = np.tile(X_res, (multiplier, 1))
y_res = np.tile(y_res, multiplier)

print(f"Final Synthetic Load Test Dataset: {len(X_res)} rows")

try:
    client = redis.Redis(
        host='localhost', 
        port=6399, 
        decode_responses=True, 
        protocol=2,
        ssl=True,
        ssl_certfile='./certs/client.crt',
        ssl_keyfile='./certs/client.key',
        ssl_cert_reqs='none'
    )
    client.ping()
except Exception as e:
    print(f"Failed to connect to DBX on localhost:6399: {e}")
    exit(1)

print("\nStarting DBX Pipeline Population...")
start_time = time.time()
pipeline = client.pipeline(transaction=False)

for i in range(len(X_res)):
    key = f"dummy:smote:feature:{i}"
    # Convert vector back to string representation to store in key-value engine
    val = ",".join(map(str, X_res[i]))
    pipeline.set(key, val)
    # Batch execute pipeline for maximum throughput in python
    if i % 2000 == 0:
        pipeline.execute()
        pipeline = client.pipeline(transaction=False)

if len(pipeline) > 0:
    pipeline.execute()

duration = time.time() - start_time
ops = len(X_res) / duration
print(f"\nPipeline Complete!")
print(f"------------------------------------------")
print(f"Total inserted: {len(X_res)} records")
print(f"Total time:     {duration:.2f} seconds")
print(f"Throughput:     {ops:.2f} Ops/sec")
print(f"------------------------------------------")
