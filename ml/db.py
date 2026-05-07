import os
import psycopg2
from dotenv import load_dotenv

load_dotenv()

def get_connection():
    """Return a psycopg2 connection from DATABASE_URL env var."""
    url = os.environ["DATABASE_URL"]
    return psycopg2.connect(url)
