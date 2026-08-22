import redis
from typing import List, Tuple

class DBXClient:
    def __init__(self, host: str = "localhost", port: int = 6399, password: str = None, 
                 ca_cert: str = None, client_cert: str = None, client_key: str = None):
        """Initialize the DBX Client using the underlying Redis protocol with mTLS."""
        connection_kwargs = {
            "host": host,
            "port": port,
            "password": password,
            "decode_responses": True,
            "protocol": 2
        }
        
        # If mTLS is enabled
        if ca_cert and client_cert and client_key:
            connection_kwargs.update({
                "ssl": True,
                "ssl_ca_certs": ca_cert,
                "ssl_certfile": client_cert,
                "ssl_keyfile": client_key,
                "ssl_cert_reqs": "none" # For self-signed dev certificates
            })
            
        self.r = redis.Redis(**connection_kwargs)

    def ping(self) -> bool:
        """Check connection to DBX server."""
        return self.r.ping()

    def vadd(self, index_name: str, doc_id: str, vector: List[float]) -> bool:
        """Add a vector to the specified index."""
        # VADD index_name doc_id f1 f2 ...
        res = self.r.execute_command("VADD", index_name, doc_id, *vector)
        return res == 1

    def vadd_batch(self, index_name: str, dim: int, doc_ids: List[str], vectors: List[List[float]]) -> int:
        """Add multiple vectors to the specified index in a single network round-trip."""
        if len(doc_ids) != len(vectors):
            raise ValueError("doc_ids and vectors must be the same length")
        
        args = [index_name, dim]
        for i in range(len(doc_ids)):
            args.append(doc_ids[i])
            args.extend(vectors[i])
            
        res = self.r.execute_command("VADD_BATCH", *args)
        return res

    def vsearch(self, index_name: str, query_vector: List[float], top_k: int = 4) -> List[Tuple[str, float]]:
        """Search the index for the closest vectors."""
        # VSEARCH index_name f1 f2 ... top_k
        args = [index_name] + query_vector + [top_k]
        res = self.r.execute_command("VSEARCH", *args)
        # res is a list of lists: [['doc_id1', '0.9'], ['doc_id2', '0.8']]
        results = []
        for item in res:
            results.append((item[0], float(item[1])))
        return results

    def get(self, key: str) -> str:
        """Retrieve a standard string key."""
        return self.r.get(key)
        
    def set(self, key: str, val: str) -> bool:
        """Set a standard string key."""
        return self.r.set(key, val)
