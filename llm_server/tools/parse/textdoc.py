import os
import io
import re
import csv
from typing import List, Dict, Any, Union

class TextDocParser:
    """Parser for text-based documents that can be read directly."""
    
    def __init__(self):
        """Initialize the text document parser."""
        # Define supported text formats
        self.supported_formats = {
            # Plain text
            '.txt': 'text/plain',
            
            # Code files
            '.py': 'text/x-python',
            '.java': 'text/x-java',
            '.js': 'text/javascript',
            '.html': 'text/html',
            '.css': 'text/css',
            '.c': 'text/x-c',
            '.cpp': 'text/x-c++',
            '.h': 'text/x-c',
            '.hpp': 'text/x-c++',
            '.cs': 'text/x-csharp',
            '.php': 'text/x-php',
            '.rb': 'text/x-ruby',
            '.go': 'text/x-go',
            '.rs': 'text/x-rust',
            '.sql': 'text/x-sql',
            '.ts': 'text/x-typescript',
            '.swift': 'text/x-swift',
            '.kt': 'text/x-kotlin',
            
            # Data files
            '.csv': 'text/csv',
            '.tsv': 'text/tab-separated-values',
            '.json': 'application/json',
            '.xml': 'text/xml',
            '.yaml': 'text/yaml',
            '.yml': 'text/yaml',
        }
    
    def parse(self, file_path: str) -> str:
        """
        Parse text-based files by directly reading them.
        
        Args:
            file_path (str): Path to the text file
            
        Returns:
            str: Content of the text file
        """
        try:
            # Get file extension and check if it's supported
            _, file_ext = os.path.splitext(file_path.lower())
            if file_ext not in self.supported_formats:
                return f"Unsupported text format: {file_ext}"
            
            # Special handling for CSV/TSV files to format them nicely
            if file_ext in ['.csv', '.tsv']:
                delimiter = ',' if file_ext == '.csv' else '\t'
                return self._parse_delimited_file(file_path, delimiter)
            
            # For all other text files, just read the content
            with open(file_path, 'r', encoding='utf-8', errors='replace') as file:
                content = file.read()
                
            return content
            
        except Exception as e:
            return f"Error processing text file: {str(e)}"
    
    def _parse_delimited_file(self, file_path: str, delimiter: str = ',') -> str:
        """
        Parse CSV or TSV files and return formatted content.
        
        Args:
            file_path (str): Path to the CSV/TSV file
            delimiter (str): Delimiter character (comma for CSV, tab for TSV)
            
        Returns:
            str: Formatted representation of the delimited file
        """
        try:
            rows = []
            with open(file_path, 'r', encoding='utf-8', errors='replace') as file:
                reader = csv.reader(file, delimiter=delimiter)
                for row in reader:
                    rows.append(row)
            
            if not rows:
                return "Empty file"
            
            # Format as a table
            col_widths = [max(len(str(row[i])) for row in rows if i < len(row)) 
                         for i in range(max(len(row) for row in rows))]
            
            formatted_rows = []
            for i, row in enumerate(rows):
                formatted_row = ' | '.join(str(cell).ljust(col_widths[j]) 
                                          for j, cell in enumerate(row))
                formatted_rows.append('| ' + formatted_row + ' |')
                
                # Add separator after header
                if i == 0:
                    sep = '|' + '|'.join('-' * (w + 2) for w in col_widths) + '|'
                    formatted_rows.append(sep)
            
            return '\n'.join(formatted_rows)
            
        except Exception as e:
            return f"Error processing delimited file: {str(e)}" 