import sys
import os

sys.path.append(os.path.join(os.path.dirname(__file__), '..', 'sdk', 'python'))
from dbx import DBXClient

cert_dir = os.path.join(os.path.dirname(__file__), '..', 'certs')
client = DBXClient(port=6399, 
                   ca_cert=os.path.join(cert_dir, "ca.crt"), 
                   client_cert=os.path.join(cert_dir, "client.crt"), 
                   client_key=os.path.join(cert_dir, "client.key"))

client.ping()
print("Ping OK")

print("VADD 1")
client.vadd("test_graph", "doc1", [1.0, 0.0, 0.0])
print("VADD 2")
client.vadd("test_graph", "doc2", [0.9, 0.1, 0.0])
print("VADD 3")
client.vadd("test_graph", "doc3", [0.0, 1.0, 0.0])
print("VADD 4")
client.vadd("test_graph", "doc4", [0.0, 0.0, 1.0])

res = client.vsearch("test_graph", [1.0, 0.0, 0.0], top_k=2)
print("Search Result:", res)
