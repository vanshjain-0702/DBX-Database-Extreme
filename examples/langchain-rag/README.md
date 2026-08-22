# LangChain RAG with DBX

This example demonstrates how to use DBX as a vector store backend for a LangChain Retrieval-Augmented Generation (RAG) pipeline.

## Prerequisites

- Python 3.10+
- DBX running locally (`make run-dev`)

## Setup

```bash
pip install langchain langchain-openai openai
pip install -e ../../sdk/python
```

## Run

```bash
export OPENAI_API_KEY="your_openai_key"
python rag_example.py
```

## Code

```python
from langchain_dbx import DBXVectorStore
from langchain_openai import OpenAIEmbeddings, ChatOpenAI
from langchain.chains import RetrievalQA

# Initialize DBX as vector store
embeddings = OpenAIEmbeddings()
db = DBXVectorStore(
    host="localhost",
    port=8000,
    tenant_id="my-app",
    token="your_jwt_token",
    embedding_function=embeddings,
)

# Add documents
db.add_texts([
    "DBX is an AI-native in-memory database.",
    "It supports both Redis-compatible KV and Vector Search.",
    "You can use it as a drop-in replacement for Redis + Pinecone.",
])

# Ask a question
qa_chain = RetrievalQA.from_chain_type(
    llm=ChatOpenAI(model="gpt-4o"),
    retriever=db.as_retriever(search_kwargs={"k": 3}),
)

answer = qa_chain.invoke("What is DBX used for?")
print(answer["result"])
```
