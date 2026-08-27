from typing import List, Optional, Tuple, Union, cast

import redis


class DBXClient:
    """RESP client for the public DBX ingress.

    The first command on :6380 must be AUTH tenantID:keyID secret.
    Point this at the orchestrator ingress, not a loopback tenant port.
    """

    def __init__(
        self,
        host: str = "localhost",
        port: int = 6380,
        tenant: Optional[str] = None,
        key_id: Optional[str] = None,
        secret: Optional[str] = None,
        password: Optional[str] = None,
        username: Optional[str] = None,
        ca_cert: Optional[str] = None,
        client_cert: Optional[str] = None,
        client_key: Optional[str] = None,
        ssl_check_hostname: bool = True,
    ) -> None:
        if tenant and key_id:
            username = f"{tenant}:{key_id}"
            if secret:
                password = secret
        use_ssl = bool(ca_cert and client_cert and client_key)
        self.r = redis.Redis(
            host=host,
            port=port,
            username=username,
            password=password,
            decode_responses=True,
            protocol=2,
            ssl=use_ssl,
            ssl_ca_certs=ca_cert if use_ssl else None,
            ssl_certfile=client_cert if use_ssl else None,
            ssl_keyfile=client_key if use_ssl else None,
            ssl_check_hostname=ssl_check_hostname if use_ssl else False,
        )

    def ping(self) -> bool:
        return bool(self.r.ping())

    def set(self, key: str, val: str, ex: Optional[int] = None) -> bool:
        return bool(self.r.set(key, val, ex=ex))

    def get(self, key: str) -> Optional[str]:
        value = self.r.get(key)
        if value is None:
            return None
        if isinstance(value, bytes):
            return value.decode("utf-8")
        return str(value)

    def delete(self, *keys: str) -> int:
        return int(self.r.delete(*keys) or 0)

    def vadd(self, index_name: str, doc_id: str, vector: List[float]) -> bool:
        res = self.r.execute_command("VADD", index_name, doc_id, *vector)
        return res == 1

    def vadd_batch(
        self, index_name: str, dim: int, doc_ids: List[str], vectors: List[List[float]]
    ) -> int:
        if len(doc_ids) != len(vectors):
            raise ValueError("doc_ids and vectors must be the same length")
        args: List[Union[str, int, float]] = [index_name, dim]
        for i, doc_id in enumerate(doc_ids):
            args.append(doc_id)
            args.extend(vectors[i])
        res = self.r.execute_command("VADD_BATCH", *args)
        return int(cast(Union[int, str], res or 0))

    def vsearch(
        self, index_name: str, query_vector: List[float], top_k: int = 4
    ) -> List[Tuple[str, float]]:
        res = self.r.execute_command("VSEARCH", index_name, *query_vector, top_k)
        results: List[Tuple[str, float]] = []
        if not res:
            return results
        for item in res:
            if isinstance(item, (list, tuple)) and len(item) >= 2:
                results.append((str(item[0]), float(item[1])))
        return results

    def vdel(self, index_name: str, doc_id: str) -> bool:
        res = self.r.execute_command("VDEL", index_name, doc_id)
        return bool(res)
