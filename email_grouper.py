import os
import imaplib
import email
from email.header import decode_header
from collections import defaultdict
from datetime import datetime, timezone
import ssl
import yaml
import sys
import argparse
from pathlib import Path
from tqdm import tqdm
import sqlite3
import json
import hashlib
import re
from textual.app import App, ComposeResult
from textual.containers import Container
from textual.widgets import DataTable, Header, Footer, Static
from textual.binding import Binding
from textual import events
from textual.widgets import TabbedContent, TabPane
from textual.reactive import reactive
from textual.widgets import ProgressBar, LoadingIndicator
from rich.progress import track
import logging
from textual.message import Message

def setup_logging():
    """Setup logging to file"""
    log_file = f"protonmail_analyzer_{datetime.now().strftime('%Y%m%d_%H%M%S')}.log"
    logging.basicConfig(
        filename=log_file,
        level=logging.DEBUG,
        format='%(asctime)s - %(levelname)s - %(message)s'
    )
    logging.info("Starting ProtonMail Analyzer")
    print(f"Logging to: {os.path.abspath(log_file)}")  # This will show before TUI starts

def extract_domain(email_address):
    """Extract second-level domain from email address"""
    try:
        # Remove any display name and get just the email part
        email_part = email_address.split('<')[-1].split('>')[0].strip()
        # Extract domain part
        domain = email_part.split('@')[-1].lower()
        # Split domain into parts
        parts = domain.split('.')
        # Handle special cases like co.uk, com.au, etc.
        if len(parts) > 2 and parts[-2] in {'co', 'com', 'org', 'gov', 'edu', 'net'}:
            return f"{parts[-3]}.{parts[-2]}.{parts[-1]}"
        elif len(parts) >= 2:
            return f"{parts[-2]}.{parts[-1]}"
        return domain
    except:
        return email_address

class EmailCache:
    def __init__(self, db_path="email_cache.db"):
        """Initialize SQLite cache database"""
        self.db_path = db_path
        self._init_db()
    
    def _init_db(self):
        """Create cache table if it doesn't exist"""
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("""
                CREATE TABLE IF NOT EXISTS emails (
                    message_id TEXT PRIMARY KEY,
                    sender TEXT,
                    subject TEXT,
                    date TEXT,
                    size INTEGER,
                    has_attachments INTEGER,
                    folder TEXT,
                    last_updated TEXT
                )
            """)
    
    def get_cached_ids(self, folder):
        """Get all cached message IDs for a folder"""
        with sqlite3.connect(self.db_path) as conn:
            cursor = conn.execute(
                "SELECT message_id FROM emails WHERE folder = ?",
                (folder,)
            )
            return {row[0] for row in cursor.fetchall()}
    
    def cache_email(self, message_id, email_data, folder):
        """Store email data in cache"""
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("""
                INSERT OR REPLACE INTO emails 
                (message_id, sender, subject, date, size, has_attachments, folder, last_updated)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            """, (
                message_id,
                email_data['sender'],
                email_data['subject'],
                email_data['date'].isoformat() if email_data['date'] else None,
                email_data['size'],
                1 if email_data['has_attachments'] else 0,
                folder,
                datetime.now(timezone.utc).isoformat()
            ))
    
    def get_all_emails(self, folder, group_by_domain=False):
        """Retrieve all cached emails for a folder"""
        emails_by_sender = defaultdict(list)
        try:
            logging.info(f"Reading from cache: {self.db_path}")
            with sqlite3.connect(self.db_path) as conn:
                cursor = conn.execute(
                    """SELECT sender, subject, date, size, has_attachments 
                       FROM emails WHERE folder = ?""",
                    (folder,)
                )
                rows = cursor.fetchall()
                logging.info(f"Found {len(rows)} emails in cache")
                
                for row in rows:
                    email_data = {
                        'subject': row[1],
                        'date': datetime.fromisoformat(row[2]) if row[2] else None,
                        'size': row[3],
                        'has_attachments': bool(row[4])
                    }
                    
                    # Group by domain if requested
                    sender_key = extract_domain(row[0]) if group_by_domain else row[0]
                    emails_by_sender[sender_key].append(email_data)
                
                logging.info(f"Grouped into {len(emails_by_sender)} senders")
                return emails_by_sender
        except Exception as e:
            logging.error(f"Error reading cache: {e}")
            raise

class EmailGroup(Static):
    """Widget to display email details"""
    def __init__(self, emails, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.emails = emails

    def compose(self) -> ComposeResult:
        table = DataTable()
        table.add_columns("Subject", "Date", "Size", "Attachments")
        
        for email in sorted(self.emails, key=lambda x: x['date'] if x['date'] else datetime.min, reverse=True):
            table.add_row(
                email['subject'],
                email['date'].strftime('%Y-%m-%d %H:%M') if email['date'] else 'N/A',
                f"{email['size'] / 1024:.1f} KB",
                "Yes" if email['has_attachments'] else "No"
            )
        yield table

class ProtonMailAnalyzer:
    def __init__(self, username, password, host="127.0.0.1", port=1143, group_by_domain=False, status_callback=None):
        """Initialize connection to ProtonMail Bridge IMAP server"""
        self.username = username
        self.password = password
        self.host = host
        self.port = port
        self._raw_emails_by_sender = defaultdict(list)  # Store raw data
        self.emails_by_sender = defaultdict(list)  # Store grouped data
        self.cache = EmailCache()
        self.group_by_domain = group_by_domain
        self.status_callback = status_callback or (lambda x: None)
        
    def connect(self):
        """Establish connection to IMAP server"""
        self.imap = imaplib.IMAP4(self.host, self.port)
        # Create SSL context that accepts self-signed certs
        context = ssl.create_default_context()
        context.check_hostname = False
        context.verify_mode = ssl.CERT_NONE
        # Enable STARTTLS with custom context
        self.imap.starttls(ssl_context=context)
        self.imap.login(self.username, self.password)
        
    async def process_emails(self, folder="INBOX"):
        """Read and process all emails from specified folder"""
        try:
            logging.info("Connecting to IMAP server...")
            self.connect()
            self.imap.select(folder)
            
            logging.info("Searching for messages...")
            _, messages = self.imap.search(None, "ALL")
            message_nums = messages[0].split()
            total_messages = len(message_nums)
            
            logging.info(f"Found {total_messages} messages")
            self.status_callback(f"Found {total_messages} emails in {folder}")
            new_messages = []
            
            # Check which messages need to be processed
            for i, message_num in enumerate(message_nums, 1):
                # Update progress percentage
                progress = (i * 100) // total_messages
                self.status_callback(f"Checking messages... [{progress}%] ({i}/{total_messages})")
                
                _, msg_data = self.imap.fetch(message_num, "(RFC822.HEADER)")
                email_header = email.message_from_bytes(msg_data[0][1])
                message_id = email_header['Message-ID']
                if not message_id:
                    message_id = hashlib.sha256(msg_data[0][1]).hexdigest()
                
                if message_id not in self.cache.get_cached_ids(folder):
                    new_messages.append((message_num, message_id))
            
            new_count = len(new_messages)
            logging.info(f"Found {new_count} new messages")
            
            if new_count > 0:
                self.status_callback(f"Processing {new_count} new emails...")
                
                # Process only new messages
                for i, (message_num, message_id) in enumerate(new_messages, 1):
                    # Update progress percentage for new messages
                    progress = (i * 100) // new_count
                    self.status_callback(f"Processing new emails... [{progress}%] ({i}/{new_count})")
                    
                    _, msg_data = self.imap.fetch(message_num, "(RFC822)")
                    email_body = msg_data[0][1]
                    message = email.message_from_bytes(email_body)
                    
                    # Extract sender
                    from_header = message["from"]
                    if from_header:
                        sender = self._decode_sender(from_header)
                        email_data = {
                            'sender': sender,
                            'subject': self._decode_subject(message['subject']),
                            'date': self._parse_date(message['date']),
                            'size': len(email_body),
                            'has_attachments': self._has_attachments(message)
                        }
                        
                        # Cache the email data
                        self.cache.cache_email(message_id, email_data, folder)
            else:
                self.status_callback("No new emails to process")
            
            logging.info("Loading from cache...")
            self.status_callback("Loading emails from cache...")
            # Load all emails from cache
            self._raw_emails_by_sender = self.cache.get_all_emails(folder, group_by_domain=False)
            # Group the data according to current mode
            self.regroup_emails()
            logging.info(f"Loaded {len(self.emails_by_sender)} senders from cache")
                    
        except Exception as e:
            logging.error(f"Error in process_emails: {e}")
            raise
        finally:
            try:
                self.imap.close()
                self.imap.logout()
            except:
                pass
    
    def _decode_sender(self, from_header):
        """Decode sender email address"""
        sender_parts = decode_header(from_header)
        sender = ""
        for part, charset in sender_parts:
            if isinstance(part, bytes):
                sender += part.decode(charset or 'utf-8')
            else:
                sender += part
        return sender
    
    def _decode_subject(self, subject):
        """Decode email subject"""
        if not subject:
            return ""
        decoded_parts = decode_header(subject)
        subject_text = ""
        for part, charset in decoded_parts:
            if isinstance(part, bytes):
                subject_text += part.decode(charset or 'utf-8')
            else:
                subject_text += part
        return subject_text
    
    def _parse_date(self, date_str):
        """Convert email date string to datetime object"""
        if date_str:
            try:
                return datetime.strptime(date_str.split(' +')[0], '%a, %d %b %Y %H:%M:%S')
            except:
                return None
        return None
    
    def _has_attachments(self, message):
        """Check if email has attachments"""
        return any(part.get_filename() for part in message.walk() 
                  if part.get_content_maintype() != 'multipart')
    
    def get_sender_statistics(self):
        """Generate statistics for each sender"""
        stats = {}
        
        for sender, emails in self.emails_by_sender.items():
            stats[sender] = {
                'total_emails': len(emails),
                'avg_size': sum(email['size'] for email in emails) / len(emails),
                'with_attachments': sum(1 for email in emails if email['has_attachments']),
                'first_email': min((email['date'] for email in emails if email['date']), default=None),
                'last_email': max((email['date'] for email in emails if email['date']), default=None)
            }
            
        return stats

    def regroup_emails(self):
        """Regroup emails based on current grouping mode"""
        self.emails_by_sender.clear()
        
        for sender, emails in self._raw_emails_by_sender.items():
            key = extract_domain(sender) if self.group_by_domain else sender
            self.emails_by_sender[key].extend(emails)

class ProtonMailTUI(App):
    CSS = """
    Screen {
        background: $surface;
    }

    Header {
        dock: top;
        background: $primary;
        color: $text;
        height: 3;
        content-align: center middle;
    }

    Footer {
        dock: bottom;
        background: $primary;
        color: $text;
        height: 3;
    }

    #status-message {
        dock: top;
        height: 3;
        content-align: center middle;
        background: $boost;
        color: $text;
        border-bottom: solid $primary;
    }

    #main-container {
        background: $panel;
        height: 1fr;
        padding: 1;
    }

    DataTable {
        width: 100%;
        height: 1fr;
        border: solid $primary;
        display: block;
    }
    """
    
    BINDINGS = [
        Binding("q", "quit", "Quit", show=True),
        Binding("d", "toggle_domain_mode", "Toggle Domain Mode", show=True),
        Binding("r", "refresh", "Refresh", show=True),
    ]

    show_domains = reactive(False)
    
    class StatusUpdate(Message):
        """Status update message"""
        def __init__(self, status: str) -> None:
            self.status = status
            super().__init__()

    def __init__(self):
        super().__init__()
        self.analyzer = None
        self.current_stats = None
    
    def compose(self) -> ComposeResult:
        """Create the TUI layout"""
        yield Header("ProtonMail Analyzer")
        yield Static("Initializing...", id="status-message")
        
        # Main container for the data
        with Container(id="main-container"):
            table = DataTable(id="summary-table")
            table.add_columns(
                "Domain/Sender",
                "Total Emails",
                "Avg Size (KB)",
                "With Attachments",
                "Date Range"
            )
            yield table
            
        yield Footer()
    
    async def on_mount(self) -> None:
        """Load initial data when app starts"""
        self.title = "ProtonMail Analyzer"
        setup_logging()
        logging.info("App mounted")
        
        # Update status synchronously first
        self.update_status("Starting up...")
        # Start the loading process in a worker
        self.run_worker(self.load_data(), name="load_data")

    def update_status(self, message: str):
        """Update the status message synchronously"""
        try:
            status = self.query_one("#status-message")
            status.update(message)
            self.refresh()
        except Exception as e:
            logging.error(f"Error updating status: {e}")

    async def load_data(self):
        """Load and display email data"""
        try:
            logging.info("Loading data...")
            self.update_status("Connecting to ProtonMail Bridge...")
            
            if not self.analyzer:
                logging.info("Creating analyzer...")
                config = load_config()
                self.analyzer = ProtonMailAnalyzer(
                    username=config['credentials']['username'],
                    password=config['credentials']['password'],
                    host=config['server']['host'],
                    port=config['server']['port'],
                    group_by_domain=self.show_domains,
                    status_callback=self.update_status
                )
            
            logging.info("Processing emails...")
            await self.analyzer.process_emails()
            logging.info("Getting statistics...")
            self.current_stats = self.analyzer.get_sender_statistics()
            
            logging.info("Updating display...")
            self.update_status("Updating display...")
            self.update_summary_table()
            
            # Debug info about table state
            table = self.query_one("#summary-table")
            logging.info(f"Table state after update: rows={table.row_count}, styles={table.styles}, display={table.styles.display}")
            
            # Force table visibility
            table.styles.display = "block"
            self.refresh(layout=True)
            
            self.update_status("Ready")
            
        except Exception as e:
            logging.error(f"Error in load_data: {e}", exc_info=True)
            self.update_status(f"Error: {str(e)}")
            raise

    def update_summary_table(self):
        """Update the summary table with current data"""
        try:
            table = self.query_one("#summary-table")
            if not table:
                logging.error("Could not find summary table")
                return
                
            # Clear existing rows
            table.clear()
            
            if not self.current_stats:
                logging.error("No stats available")
                return
            
            logging.info(f"Updating table with {len(self.current_stats)} entries")
            
            sorted_stats = sorted(
                self.current_stats.items(),
                key=lambda x: x[1]['total_emails'],
                reverse=True
            )
            
            for sender, stats in sorted_stats:
                date_range = "N/A"
                if stats['first_email'] and stats['last_email']:
                    if stats['first_email'].year == stats['last_email'].year:
                        date_range = f"{stats['first_email'].strftime('%b')} - {stats['last_email'].strftime('%b %Y')}"
                    else:
                        date_range = f"{stats['first_email'].strftime('%b %Y')} - {stats['last_email'].strftime('%b %Y')}"
                
                table.add_row(
                    sender,
                    str(stats['total_emails']),
                    f"{stats['avg_size'] / 1024:.1f}",
                    str(stats['with_attachments']),
                    date_range
                )
            
            self.refresh(layout=True)
            
        except Exception as e:
            logging.error(f"Error updating table: {str(e)}")
            raise
    
    def on_data_table_row_selected(self, event: DataTable.RowSelected) -> None:
        """Handle row selection in the summary table"""
        table = event.data_table
        sender = table.get_cell_at((event.row_key, 0))
        
        # Get emails for this sender
        emails = self.analyzer.emails_by_sender[sender]
        
        # Update details table
        details_table = self.query_one("#email-details DataTable")
        details_table.clear()
        
        for email in sorted(emails, key=lambda x: x['date'] if x['date'] else datetime.min, reverse=True):
            details_table.add_row(
                email['subject'],
                email['date'].strftime('%Y-%m-%d %H:%M') if email['date'] else 'N/A',
                f"{email['size'] / 1024:.1f} KB",
                "Yes" if email['has_attachments'] else "No"
            )
    
    def action_toggle_domain_mode(self) -> None:
        """Toggle between domain and email address mode"""
        self.show_domains = not self.show_domains
        if self.analyzer:
            self.update_status("Regrouping emails...")
            self.analyzer.group_by_domain = self.show_domains
            self.analyzer.regroup_emails()
            self.current_stats = self.analyzer.get_sender_statistics()
            self.update_summary_table()
            self.update_status("Ready")
    
    def action_refresh(self) -> None:
        """Refresh email data"""
        self.run_worker(self.load_data())

def check_file_permissions(config_path):
    """Check if the config file has secure permissions"""
    if os.name == 'posix':  # Unix-like systems
        mode = os.stat(config_path).st_mode
        if mode & 0o077:  # Check if group or others have any permissions
            print("Warning: Config file has loose permissions. "
                  "Consider running: chmod 600 proton.yaml")

def load_config(config_path="proton.yaml"):
    """Load configuration from YAML file"""
    check_file_permissions(config_path)
    try:
        with open(config_path, 'r') as file:
            config = yaml.safe_load(file)
            
        # Validate required fields
        required_fields = {
            'credentials': ['username', 'password'],
            'server': ['host', 'port']
        }
        
        for section, fields in required_fields.items():
            if section not in config:
                raise ValueError(f"Missing '{section}' section in config file")
            for field in fields:
                if field not in config[section]:
                    raise ValueError(f"Missing '{field}' in '{section}' section")
        
        return config
    except FileNotFoundError:
        print(f"Error: Configuration file '{config_path}' not found.")
        print("Please create a proton.yaml file with your credentials.")
        sys.exit(1)
    except yaml.YAMLError as e:
        print(f"Error parsing YAML file: {e}")
        sys.exit(1)
    except ValueError as e:
        print(f"Error in configuration file: {e}")
        sys.exit(1)

def main():
    # Create and run TUI instead of command line interface
    app = ProtonMailTUI()
    app.run()

if __name__ == "__main__":
    main() 