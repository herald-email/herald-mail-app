import os
import imaplib
import email
import sys
import sqlite3
import hashlib
import logging
from email.header import decode_header
from collections import defaultdict
from datetime import datetime, timezone
import ssl
import yaml
from textual.app import App, ComposeResult
from textual.containers import Container, Horizontal, Vertical
from textual.widgets import DataTable, Header, Footer, Static, ProgressBar, Label
from textual.binding import Binding
from textual.reactive import reactive
from textual.message import Message
import time


def setup_logging():
    """Setup logging to file"""
    log_file = f"protonmail_analyzer_{datetime.now().strftime('%Y%m%d_%H%M%S')}.log"
    logging.basicConfig(
        filename=log_file,
        level=logging.DEBUG,
        format="%(asctime)s - %(levelname)s - %(message)s",
    )
    logging.info("Starting ProtonMail Analyzer")
    print(
        f"Logging to: {os.path.abspath(log_file)}"
    )  # This will show before TUI starts


def extract_domain(email_address):
    """Extract second-level domain from email address"""
    try:
        if not email_address or not isinstance(email_address, str):
            return email_address
        
        # Remove any display name and get just the email part
        email_part = email_address.split("<")[-1].split(">")[0].strip()
        
        # Validate email format
        if "@" not in email_part:
            return email_address
            
        # Extract domain part
        domain = email_part.split("@")[-1].lower()
        
        # Split domain into parts
        parts = domain.split(".")
        
        # Handle special cases like co.uk, com.au, etc.
        if len(parts) > 2 and parts[-2] in {"co", "com", "org", "gov", "edu", "net"}:
            return f"{parts[-3]}.{parts[-2]}.{parts[-1]}"
        elif len(parts) >= 2:
            return f"{parts[-2]}.{parts[-1]}"
        return domain
    except (IndexError, AttributeError, ValueError) as e:
        logging.warning(f"Error extracting domain from '{email_address}': {e}")
        return email_address


class EmailCache:
    def __init__(self, db_path="email_cache.db"):
        """Initialize SQLite cache database"""
        self.db_path = db_path
        self._init_db()

    def _init_db(self):
        """Create cache table if it doesn't exist"""
        with sqlite3.connect(self.db_path) as conn:
            conn.execute(
                """
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
            """
            )

    def get_cached_ids(self, folder):
        """Get all cached message IDs for a folder"""
        with sqlite3.connect(self.db_path) as conn:
            cursor = conn.execute(
                "SELECT message_id FROM emails WHERE folder = ?", (folder,)
            )
            return {row[0] for row in cursor.fetchall()}

    def cache_email(self, message_id, email_data, folder):
        """Store email data in cache"""
        with sqlite3.connect(self.db_path) as conn:
            conn.execute(
                """
                INSERT OR REPLACE INTO emails 
                (message_id, sender, subject, date, size, has_attachments, folder, last_updated)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            """,
                (
                    message_id,
                    email_data["sender"],
                    email_data["subject"],
                    email_data["date"].isoformat() if email_data["date"] else None,
                    email_data["size"],
                    1 if email_data["has_attachments"] else 0,
                    folder,
                    datetime.now(timezone.utc).isoformat(),
                ),
            )

    def get_all_emails(self, folder, group_by_domain=False):
        """Retrieve all cached emails for a folder"""
        emails_by_sender = defaultdict(list)
        try:
            logging.info(f"Reading from cache: {self.db_path}")
            with sqlite3.connect(self.db_path) as conn:
                cursor = conn.execute(
                    """SELECT sender, subject, date, size, has_attachments 
                       FROM emails WHERE folder = ?""",
                    (folder,),
                )
                rows = cursor.fetchall()
                logging.info(f"Found {len(rows)} emails in cache")

                for row in rows:
                    email_data = {
                        "subject": row[1],
                        "date": datetime.fromisoformat(row[2]) if row[2] else None,
                        "size": row[3],
                        "has_attachments": bool(row[4]),
                    }

                    # Group by domain if requested
                    sender_key = extract_domain(row[0]) if group_by_domain else row[0]
                    emails_by_sender[sender_key].append(email_data)

                logging.info(f"Grouped into {len(emails_by_sender)} senders")
                return emails_by_sender
        except Exception as e:
            logging.error(f"Error reading cache: {e}")
            raise

    def delete_sender_emails(self, sender: str, folder: str):
        """Delete all emails from a specific sender"""
        with sqlite3.connect(self.db_path) as conn:
            conn.execute(
                "DELETE FROM emails WHERE sender = ? AND folder = ?", (sender, folder)
            )

    def delete_email(self, message_id: str):
        """Delete a specific email by message ID"""
        with sqlite3.connect(self.db_path) as conn:
            conn.execute("DELETE FROM emails WHERE message_id = ?", (message_id,))


class EmailGroup(Static):
    """Widget to display email details"""

    def __init__(self, emails, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.emails = emails

    def compose(self) -> ComposeResult:
        table = DataTable()
        table.add_columns("Subject", "Date", "Size", "Attachments")

        for email in sorted(
            self.emails,
            key=lambda x: x["date"] if x["date"] else datetime.min,
            reverse=True,
        ):
            table.add_row(
                email["subject"],
                email["date"].strftime("%Y-%m-%d %H:%M") if email["date"] else "N/A",
                f"{email['size'] / 1024:.1f} KB",
                "Yes" if email["has_attachments"] else "No",
            )
        yield table


class ProtonMailAnalyzer:
    def __init__(
        self,
        username,
        password,
        host="127.0.0.1",
        port=1143,
        group_by_domain=False,
        status_callback=None,
    ):
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
        self.total_messages = 0
        self.processed_messages = 0
    
    def _cleanup_connection(self):
        """Safely cleanup IMAP connection"""
        if hasattr(self, 'imap') and self.imap:
            try:
                self.imap.close()
                logging.debug("IMAP folder closed")
            except (imaplib.IMAP4.error, AttributeError) as e:
                logging.warning(f"Error closing IMAP folder: {e}")
            
            try:
                self.imap.logout()
                logging.debug("IMAP logout successful")
            except (imaplib.IMAP4.error, AttributeError) as e:
                logging.warning(f"Error during IMAP logout: {e}")
            
            self.imap = None

    def connect(self):
        """Establish connection to IMAP server"""
        try:
            # Set socket timeout for connection
            import socket
            socket.setdefaulttimeout(30)
            
            self.imap = imaplib.IMAP4(self.host, self.port)
            # Create SSL context - disabled cert verification for localhost/ProtonMail Bridge
            context = ssl.create_default_context()
            context.check_hostname = False  # Required for localhost connections
            context.verify_mode = ssl.CERT_NONE  # Required for self-signed certs
            # Enable STARTTLS with custom context
            self.imap.starttls(ssl_context=context)
            self.imap.login(self.username, self.password)
            logging.info(f"Successfully connected to IMAP server at {self.host}:{self.port}")
        except imaplib.IMAP4.error as e:
            logging.error(f"IMAP authentication error: {e}")
            raise
        except ssl.SSLError as e:
            logging.error(f"SSL connection error: {e}")
            raise
        except socket.timeout as e:
            logging.error(f"Connection timeout: {e}")
            raise
        except Exception as e:
            logging.error(f"Connection error: {e}")
            raise

    async def process_emails(self, folder="INBOX"):
        """Read and process all emails from specified folder"""
        try:
            logging.info("Connecting to IMAP server...")
            self.connect()
            self.imap.select(folder)

            logging.info("Searching for messages...")
            _, messages = self.imap.search(None, "ALL")
            message_nums = messages[0].split()
            self.total_messages = len(message_nums)

            logging.info(f"Found {self.total_messages} messages")
            self.status_callback(f"Found {self.total_messages} emails in {folder}")
            new_messages = []

            # Check which messages need to be processed
            for i, message_num in enumerate(message_nums, 1):
                # Update progress percentage
                progress = (i * 100) // self.total_messages
                self.status_callback(
                    f"Scanning messages... [{progress}%] ({i}/{self.total_messages})"
                )

                _, msg_data = self.imap.fetch(message_num, "(RFC822.HEADER)")
                email_header = email.message_from_bytes(msg_data[0][1])
                message_id = email_header["Message-ID"]
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
                    self.processed_messages = i
                    self.status_callback(
                        f"Processing emails... [{progress}%] ({i}/{new_count})"
                    )

                    _, msg_data = self.imap.fetch(message_num, "(RFC822)")
                    if not msg_data or not msg_data[0] or not msg_data[0][1]:
                        logging.warning(f"No message data for message {message_num}")
                        continue
                        
                    email_body = msg_data[0][1]
                    if isinstance(email_body, bytes):
                        message = email.message_from_bytes(email_body)
                    else:
                        logging.warning(f"Invalid email body type for message {message_num}: {type(email_body)}")
                        continue

                    # Extract sender - validate message exists
                    if not message:
                        logging.warning(f"Empty message for ID {message_id}")
                        continue
                        
                    from_header = message.get("from")
                    if from_header:
                        sender = self._decode_sender(from_header)
                        
                        # Validate sender is not empty
                        if not sender or not sender.strip():
                            logging.warning(f"Empty sender for message {message_id}")
                            continue
                            
                        email_data = {
                            "sender": sender,
                            "subject": self._decode_subject(message.get("subject")),
                            "date": self._parse_date(message.get("date")),
                            "size": len(email_body) if isinstance(email_body, (bytes, str)) else 0,
                            "has_attachments": self._has_attachments(message),
                        }

                        # Cache the email data
                        self.cache.cache_email(message_id, email_data, folder)
                    else:
                        logging.warning(f"No sender found for message {message_id}")
            else:
                self.status_callback("No new emails to process")

            logging.info("Loading from cache...")
            self.status_callback("Loading emails from cache...")
            # Load all emails from cache
            self._raw_emails_by_sender = self.cache.get_all_emails(
                folder, group_by_domain=False
            )
            # Group the data according to current mode
            self.regroup_emails()
            logging.info(f"Loaded {len(self.emails_by_sender)} senders from cache")

        except Exception as e:
            logging.error(f"Error in process_emails: {e}")
            raise
        finally:
            self._cleanup_connection()

    def _decode_sender(self, from_header):
        """Decode sender email address"""
        if not from_header:
            return ""
        
        try:
            sender_parts = decode_header(from_header)
            sender = ""
            for part, charset in sender_parts:
                if isinstance(part, bytes):
                    try:
                        sender += part.decode(charset or "utf-8", errors='replace')
                    except (UnicodeDecodeError, LookupError):
                        # Fallback to utf-8 with error replacement
                        sender += part.decode("utf-8", errors='replace')
                else:
                    sender += str(part)
            return sender
        except Exception as e:
            logging.warning(f"Error decoding sender header '{from_header}': {e}")
            return str(from_header)

    def _decode_subject(self, subject):
        """Decode email subject"""
        if not subject:
            return ""
        
        try:
            decoded_parts = decode_header(subject)
            subject_text = ""
            for part, charset in decoded_parts:
                if isinstance(part, bytes):
                    try:
                        subject_text += part.decode(charset or "utf-8", errors='replace')
                    except (UnicodeDecodeError, LookupError):
                        # Fallback to utf-8 with error replacement
                        subject_text += part.decode("utf-8", errors='replace')
                else:
                    subject_text += str(part)
            return subject_text
        except Exception as e:
            logging.warning(f"Error decoding subject '{subject}': {e}")
            return str(subject)

    def _parse_date(self, date_str):
        """Convert email date string to datetime object"""
        if not date_str:
            return None
            
        try:
            # Try standard email date format first
            return datetime.strptime(
                date_str.split(" +")[0], "%a, %d %b %Y %H:%M:%S"
            )
        except ValueError:
            # Try alternative formats
            try:
                return datetime.strptime(
                    date_str.split(" +")[0], "%d %b %Y %H:%M:%S"
                )
            except ValueError:
                try:
                    # Try ISO format
                    return datetime.fromisoformat(date_str.replace('Z', '+00:00'))
                except ValueError as e:
                    logging.warning(f"Could not parse date '{date_str}': {e}")
                    return None

    def _has_attachments(self, message):
        """Check if email has attachments"""
        return any(
            part.get_filename()
            for part in message.walk()
            if part.get_content_maintype() != "multipart"
        )

    def get_sender_statistics(self):
        """Generate statistics for each sender"""
        stats = {}

        for sender, emails in self.emails_by_sender.items():
            if not emails:  # Skip empty lists
                continue

            stats[sender] = {
                "total_emails": len(emails),
                "avg_size": sum(email["size"] for email in emails)
                / max(len(emails), 1),  # Avoid division by zero
                "with_attachments": sum(
                    1 for email in emails if email["has_attachments"]
                ),
                "first_email": min(
                    (email["date"] for email in emails if email["date"]), default=None
                ),
                "last_email": max(
                    (email["date"] for email in emails if email["date"]), default=None
                ),
            }

        return stats

    def regroup_emails(self):
        """Regroup emails based on current grouping mode"""
        self.emails_by_sender.clear()

        # Skip empty senders
        for sender, emails in self._raw_emails_by_sender.items():
            if emails:  # Only add non-empty lists
                key = extract_domain(sender) if self.group_by_domain else sender
                self.emails_by_sender[key].extend(emails)

    async def delete_sender_emails(self, sender: str, folder="INBOX"):
        """Delete all emails from a sender in both IMAP and cache"""
        try:
            logging.info(f"Starting deletion of all emails from {sender}")
            self.connect()
            self.imap.select(folder)

            # Extract email address if in "Name <email>" format
            original_sender = sender
            if "<" in sender:
                sender = sender.split("<")[1].split(">")[0]
                logging.info(
                    f"Extracted email address: {sender} from {original_sender}"
                )

            # Use simpler search criteria
            search_criteria = f"FROM {sender}"
            logging.info(f"Searching with criteria: {search_criteria}")
            _, messages = self.imap.search(None, search_criteria)
            message_nums = messages[0].split()

            logging.info(f"Found {len(message_nums)} messages to delete")
            if message_nums:
                # Delete all matching emails
                for num in message_nums:
                    logging.info(f"Deleting message number {num}")
                    self.imap.store(num, "+FLAGS", "\\Deleted")
                self.imap.expunge()
                logging.info("Expunged deleted messages")

            # Delete from cache
            logging.info("Deleting from cache")
            self.cache.delete_sender_emails(original_sender, folder)

            # Remove from both raw and grouped data
            logging.info("Removing from memory")
            if original_sender in self._raw_emails_by_sender:
                del self._raw_emails_by_sender[original_sender]
                logging.info("Removed from raw data")
            if original_sender in self.emails_by_sender:
                del self.emails_by_sender[original_sender]
                logging.info("Removed from grouped data")

            logging.info(f"Successfully completed deletion for {original_sender}")

        except Exception as e:
            logging.error(f"Error in delete_sender_emails: {e}", exc_info=True)
            raise
        finally:
            self._cleanup_connection()

    async def delete_email(self, sender: str, email_data: dict, folder="INBOX"):
        """Delete specific email in both IMAP and cache"""
        try:
            self.connect()
            self.imap.select(folder)

            # Extract just the email address if in "Name <email>" format
            if "<" in sender:
                sender = sender.split("<")[1].split(">")[0]

            # Use simpler search criteria - just FROM and DATE
            if email_data["date"]:
                date_str = email_data["date"].strftime("%d-%b-%Y")
                search_criteria = f"(FROM {sender} ON {date_str})"
            else:
                search_criteria = f"(FROM {sender})"

            _, messages = self.imap.search(None, search_criteria)
            message_nums = messages[0].split()

            if message_nums:
                # Delete first matching email
                self.imap.store(message_nums[0], "+FLAGS", "\\Deleted")
                self.imap.expunge()

            # Remove from memory
            emails = self.emails_by_sender[sender]
            if email_data in emails:
                emails.remove(email_data)

        finally:
            self._cleanup_connection()

    async def cleanup_cache(self, folder="INBOX"):
        """Sync cache with IMAP server by removing deleted messages"""
        try:
            logging.info("Starting cache cleanup...")
            self.connect()
            self.imap.select(folder)

            # Get all cached message IDs
            cached_ids = self.cache.get_cached_ids(folder)
            logging.info(f"Found {len(cached_ids)} messages in cache")

            # Get all current IMAP message IDs
            _, messages = self.imap.search(None, "ALL")
            message_nums = messages[0].split()
            current_ids = set()

            # Fetch Message-IDs from IMAP
            for num in message_nums:
                _, msg_data = self.imap.fetch(num, "(RFC822.HEADER)")
                email_header = email.message_from_bytes(msg_data[0][1])
                message_id = email_header["Message-ID"]
                if not message_id:
                    message_id = hashlib.sha256(msg_data[0][1]).hexdigest()
                current_ids.add(message_id)

            logging.info(f"Found {len(current_ids)} messages on IMAP server")

            # Find deleted messages
            deleted_ids = cached_ids - current_ids
            if deleted_ids:
                logging.info(
                    f"Found {len(deleted_ids)} deleted messages to remove from cache"
                )
                # Remove deleted messages from cache
                for message_id in deleted_ids:
                    self.cache.delete_email(message_id)
                logging.info("Cache cleanup completed")
            else:
                logging.info("No deleted messages found in cache")

        except Exception as e:
            logging.error(f"Error in cleanup_cache: {e}", exc_info=True)
            raise
        finally:
            self._cleanup_connection()


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
        height: 1;
        content-align: center middle;
        background: $boost;
        color: $text;
        border-bottom: solid $primary;
    }

    #loading-container {
        dock: top;
        height: 7;
        background: $surface;
        border: solid $primary;
        margin: 1;
        padding: 1;
    }

    #loading-banner {
        content-align: center middle;
        background: $primary;
        color: $text;
        height: 3;
        text-style: bold;
        margin-bottom: 1;
    }

    #progress-info {
        content-align: center middle;
        height: 1;
        color: $text;
    }

    #loading-spinner {
        content-align: center middle;
        height: 1;
        color: $accent;
    }

    #main-container {
        layout: grid;
        grid-size: 2;  /* Two columns */
        grid-columns: 3fr 2fr;  /* Left panel wider than right */
        padding: 1;
    }

    #summary-table {
        width: 100%;
        height: 100%;
        border: solid $primary;
        display: block;
        overflow: scroll;  /* Change to scroll */
        min-width: 50;
        scrollbar-size: 1 2;  /* Horizontal scrollbar more visible */
        scrollbar-gutter: stable;
        overflow-y: scroll;
        overflow-x: scroll;
    }

    #summary-table > .datatable--cursor {
        background: $accent;
        color: $text;
    }

    #summary-table:focus > .datatable--cursor {
        background: $accent;
        color: $text;
        text-style: bold;
    }

    #details-table {
        width: 100%;
        height: 100%;
        border: solid $primary;
        display: block;
        margin-left: 1;
        overflow: scroll;  /* Change to scroll */
        min-width: 50;
        scrollbar-size: 1 2;  /* Horizontal scrollbar more visible */
        scrollbar-gutter: stable;
        overflow-y: scroll;
        overflow-x: scroll;
    }

    #details-table > .datatable--header {
        background: $primary;
        color: $text;
    }

    #details-table > .datatable--cursor {
        background: $accent;
        color: $text;
    }

    #details-table:focus > .datatable--cursor {
        background: $accent;
        color: $text;
        text-style: bold;
    }

    #summary-table .selected {
        background: $accent-darken-2;
        color: $text;
        text-style: bold;
    }
    """

    BINDINGS = [
        Binding("q", "quit", "Quit", show=True),
        Binding("d", "toggle_domain_mode", "Toggle Domain Mode", show=True),
        Binding("r", "refresh", "Refresh", show=True),
        Binding("D", "delete_selected", "Delete Selected", show=True),
        Binding("space", "toggle_selection", "Toggle Selection", show=True),
    ]

    show_domains = reactive(False)
    selected_rows = set()  # Track selected rows

    class StatusUpdate(Message):
        """Status update message"""

        def __init__(self, status: str) -> None:
            self.status = status
            super().__init__()

    def __init__(self):
        super().__init__()
        self.analyzer: ProtonMailAnalyzer | None = None
        self.current_stats = None
        self.loading = True
        self.spinner_chars = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"]
        self.spinner_index = 0
        self.start_time = None

    def compose(self) -> ComposeResult:
        """Create the TUI layout"""
        yield Header("ProtonMail Analyzer")
        yield Static("Initializing...", id="status-message")

        # Loading container (shown during initial load)
        with Container(id="loading-container"):
            yield Static("🔍 Loading Email Data...", id="loading-banner")
            yield Static("", id="progress-info")
            yield Static("⠋", id="loading-spinner")

        # Main container with two columns (hidden during loading)
        with Container(id="main-container"):
            # Summary table (left side)
            table = DataTable(id="summary-table", cursor_type="row")
            table.add_column("Domain/Sender", width=40)
            table.add_columns(
                "Total Emails", "Avg Size (KB)", "With Attachments", "Date Range"
            )
            table.can_focus = True
            yield table

            # Details table (right side)
            details = DataTable(id="details-table", cursor_type="row")
            details.add_column("Date", width=20)
            details.add_column("Subject", width=40)
            details.add_columns("Size", "Attachments")
            details.can_focus = True
            yield details

        yield Footer()

    def on_mount(self) -> None:
        """Load initial data when app starts"""
        self.title = "ProtonMail Analyzer"
        setup_logging()
        logging.info("App mounted")
        
        # Hide main container and show loading container initially
        self.query_one("#main-container").styles.display = "none"
        self.query_one("#loading-container").styles.display = "block"
        
        # Start spinner animation
        self.start_time = time.time()
        self.set_interval(0.1, self.update_spinner)
        
        # Update status synchronously first
        self.update_status("Starting up...")
        # Start the loading process in a worker
        self.run_worker(self.load_data(), name="load_data")

    def update_spinner(self):
        """Update the loading spinner animation"""
        if self.loading:
            try:
                spinner = self.query_one("#loading-spinner")
                self.spinner_index = (self.spinner_index + 1) % len(self.spinner_chars)
                spinner.update(self.spinner_chars[self.spinner_index])
                
                # Update elapsed time and progress info
                if self.start_time:
                    elapsed = time.time() - self.start_time
                    progress_info = self.query_one("#progress-info")
                    
                    # Show more detailed progress if analyzer is available
                    if self.analyzer and hasattr(self.analyzer, 'processed_messages') and self.analyzer.total_messages > 0:
                        processed = getattr(self.analyzer, 'processed_messages', 0)
                        total = getattr(self.analyzer, 'total_messages', 1)
                        percent = (processed * 100) // total if total > 0 else 0
                        
                        # Calculate estimated time remaining
                        if processed > 0:
                            avg_time_per_message = elapsed / processed
                            remaining_messages = total - processed
                            eta_seconds = avg_time_per_message * remaining_messages
                            eta_str = f" | ETA: {eta_seconds:.0f}s" if eta_seconds > 0 else ""
                        else:
                            eta_str = ""
                            
                        progress_info.update(f"Progress: {processed}/{total} ({percent}%) | Elapsed: {elapsed:.1f}s{eta_str}")
                    else:
                        progress_info.update(f"Elapsed time: {elapsed:.1f}s")
                    
            except Exception as e:
                logging.error(f"Error updating spinner: {e}")

    def update_status(self, message: str):
        """Update the status message synchronously"""
        try:
            status = self.query_one("#status-message")
            status.update(message)
            
            # Also update loading banner if in loading mode
            if self.loading:
                try:
                    banner = self.query_one("#loading-banner")
                    if "emails" in message.lower() or "processing" in message.lower():
                        banner.update(f"📧 {message}")
                    elif "connecting" in message.lower():
                        banner.update(f"🔗 {message}")
                    elif "cleaning" in message.lower():
                        banner.update(f"🧹 {message}")
                    else:
                        banner.update(f"⚙️ {message}")
                except Exception:
                    pass  # Banner might not exist yet
                    
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
                    username=config["credentials"]["username"],
                    password=config["credentials"]["password"],
                    host=config["server"]["host"],
                    port=config["server"]["port"],
                    group_by_domain=self.show_domains,
                    status_callback=self.update_status,
                )

            # Run cache cleanup before processing
            self.update_status("Cleaning up cache...")
            await self.analyzer.cleanup_cache()

            logging.info("Processing emails...")
            await self.analyzer.process_emails()
            logging.info("Getting statistics...")
            self.current_stats = self.analyzer.get_sender_statistics()

            logging.info("Updating display...")
            self.update_status("Updating display...")
            self.update_summary_table()

            # Debug info about table state
            table = self.query_one("#summary-table")
            logging.info(
                f"Table state after update: rows={table.row_count}, styles={table.styles}, display={table.styles.display}"
            )

            # Hide loading container and show main container
            self.loading = False
            self.query_one("#loading-container").styles.display = "none"
            self.query_one("#main-container").styles.display = "block"
            
            # Make summary table focusable
            table = self.query_one("#summary-table")
            table.focus()
            
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
                key=lambda x: x[1]["total_emails"],
                reverse=True,
            )

            for sender, stats in sorted_stats:
                date_range = "N/A"
                if stats["first_email"] and stats["last_email"]:
                    if stats["first_email"].year == stats["last_email"].year:
                        date_range = f"{stats['first_email'].strftime('%b')} - {stats['last_email'].strftime('%b %Y')}"
                    else:
                        date_range = f"{stats['first_email'].strftime('%b %Y')} - {stats['last_email'].strftime('%b %Y')}"

                table.add_row(
                    sender,
                    str(stats["total_emails"]),
                    f"{stats['avg_size'] / 1024:.1f}",
                    str(stats["with_attachments"]),
                    date_range,
                )

            self.refresh(layout=True)

        except Exception as e:
            logging.error(f"Error updating table: {str(e)}")
            raise

    def on_data_table_row_selected(self, event: DataTable.RowSelected) -> None:
        """Handle row selection in the summary table"""
        if event.data_table.id != "summary-table":
            return

        try:
            # Convert RowKey to integer index
            sender = event.data_table.get_row(event.row_key)[0]
            logging.info(f"Selected sender: {sender}")

            # Get emails for this sender
            emails = self.analyzer.emails_by_sender[sender]
            logging.info(f"Found {len(emails)} emails for sender")

            # Update details table
            details_table = self.query_one("#details-table")
            details_table.clear()

            # Sort emails by date
            sorted_emails = sorted(
                emails,
                key=lambda x: x["date"] if x["date"] else datetime.min,
                reverse=True,
            )

            for email in sorted_emails:
                details_table.add_row(
                    (
                        email["date"].strftime("%Y-%m-%d %H:%M")
                        if email["date"]
                        else "N/A"
                    ),  # Date first
                    email["subject"] or "No Subject",
                    f"{email['size'] / 1024:.1f} KB",
                    "Yes" if email["has_attachments"] else "No",
                )

            details_table.refresh()
            self.refresh(layout=True)

            self.update_status(f"Showing {len(emails)} emails from {sender}")
            logging.info(f"Updated details table with {len(sorted_emails)} rows")

        except Exception as e:
            logging.error(f"Error showing email details: {e}", exc_info=True)
            self.update_status(f"Error showing details: {str(e)}")

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

    def action_toggle_selection(self) -> None:
        """Toggle selection of current row"""
        focused = self.focused
        if (
            not focused
            or not isinstance(focused, DataTable)
            or focused.id != "summary-table"
        ):
            return

        if focused.cursor_row is not None:
            row_data = focused.get_row_at(focused.cursor_row)
            if not row_data:
                return

            sender = row_data[0]
            # Remove any existing Rich markup
            clean_sender = sender.replace("[reverse]", "").replace("[/]", "")

            if focused.cursor_row in self.selected_rows:
                # Deselect: Remove from set and remove highlighting
                self.selected_rows.remove(focused.cursor_row)
                focused.update_cell_at((focused.cursor_row, 0), clean_sender)
            else:
                # Select: Add to set and add highlighting
                self.selected_rows.add(focused.cursor_row)
                focused.update_cell_at(
                    (focused.cursor_row, 0), f"[reverse]{clean_sender}[/]"
                )

            self.update_status(f"Selected {len(self.selected_rows)} groups")

    async def action_delete_selected(self) -> None:
        """Delete selected groups or message"""
        focused = self.focused
        if not focused or not isinstance(focused, DataTable):
            return

        try:
            if focused.id == "summary-table":
                if self.selected_rows:  # Handle multiple selections
                    total = len(self.selected_rows)
                    self.update_status(f"Deleting {total} selected groups...")

                    # Convert to list and sort in reverse to avoid index shifting
                    rows_to_delete = []
                    for row_idx in sorted(list(self.selected_rows), reverse=True):
                        row = focused.get_row_at(row_idx)
                        if row:
                            sender = row[0]
                            logging.info(f"Queuing deletion of {sender}")
                            rows_to_delete.append((row_idx, sender))

                    # Delete each group
                    for row_idx, sender in rows_to_delete:
                        logging.info(f"Deleting {sender}")
                        await self.analyzer.delete_sender_emails(sender)
                        # Remove row from UI immediately
                        focused.remove_row(row_idx)

                    # Clear selections
                    self.selected_rows.clear()

                    # Force a complete reload of data
                    logging.info("Forcing complete data reload")
                    await self.load_data()
                    self.update_status(f"Deleted {total} groups")

                elif focused.cursor_row is not None:  # Single selection delete
                    row = focused.get_row_at(focused.cursor_row)
                    if row:
                        sender = row[0]
                        self.update_status(f"Deleting all emails from {sender}...")
                        await self.analyzer.delete_sender_emails(sender)
                        self.current_stats = self.analyzer.get_sender_statistics()
                        self.update_summary_table()
                        self.query_one("#details-table").clear()
                        self.update_status(f"Deleted all emails from {sender}")

            elif focused.id == "details-table":
                if focused.cursor_row is not None:
                    summary_table = self.query_one("#summary-table")
                    row = summary_table.get_row_at(summary_table.cursor_row)
                    if row:
                        sender = row[0]
                        emails = self.analyzer.emails_by_sender[sender]
                        if focused.cursor_row < len(emails):
                            email_data = emails[focused.cursor_row]
                            self.update_status(f"Deleting email...")
                            await self.analyzer.delete_email(sender, email_data)
                            self.current_stats = self.analyzer.get_sender_statistics()
                            self.update_summary_table()

                            # Refresh details view directly instead of using event
                            details_table = self.query_one("#details-table")
                            details_table.clear()

                            # Get updated emails list
                            updated_emails = self.analyzer.emails_by_sender[sender]
                            sorted_emails = sorted(
                                updated_emails,
                                key=lambda x: x["date"] if x["date"] else datetime.min,
                                reverse=True,
                            )

                            for email in sorted_emails:
                                details_table.add_row(
                                    (
                                        email["date"].strftime("%Y-%m-%d %H:%M")
                                        if email["date"]
                                        else "N/A"
                                    ),
                                    email["subject"] or "No Subject",
                                    f"{email['size'] / 1024:.1f} KB",
                                    "Yes" if email["has_attachments"] else "No",
                                )

                            self.update_status(f"Deleted email from {sender}")

        except Exception as e:
            logging.error(f"Error deleting: {e}", exc_info=True)
            self.update_status(f"Error deleting: {str(e)}")


def check_file_permissions(config_path):
    """Check if the config file has secure permissions"""
    if os.name == "posix":  # Unix-like systems
        try:
            mode = os.stat(config_path).st_mode
            if mode & 0o077:  # Check if group or others have any permissions
                logging.warning(
                    f"Config file '{config_path}' has loose permissions (mode: {oct(mode)}). "
                    "Consider running: chmod 600 proton.yaml"
                )
                print(
                    "Warning: Config file has loose permissions. "
                    "Consider running: chmod 600 proton.yaml"
                )
        except OSError as e:
            logging.error(f"Could not check permissions for '{config_path}': {e}")
            raise


def load_config(config_path="proton.yaml"):
    """Load configuration from YAML file"""
    check_file_permissions(config_path)
    try:
        with open(config_path, "r") as file:
            config = yaml.safe_load(file)

        # Validate required fields
        required_fields = {
            "credentials": ["username", "password"],
            "server": ["host", "port"],
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
