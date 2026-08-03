%{
package cmdwire
%}

%union {
	text   string
	field  Field
	fields []Field
	tail   parsedTail
}

%token REQUEST OK_TOKEN ERR NOTICE_TOKEN EVENT_TOKEN ITEM_TOKEN CHUNK_TOKEN
%token <text> COMMAND_TOKEN PATH_TOKEN
%token <field> FIELD_TOKEN CODE_FIELD SCHEMA_FIELD COUNT_FIELD

%type <fields> fields nonempty_fields
%type <tail> optional_tail data_tail

%%

input:
	REQUEST COMMAND_TOKEN optional_tail
	{
		setParsedRecord(cwlex, Record{
			Kind: Request, Command: $2, Resource: $3.resource, Fields: $3.fields,
		})
	}
	| OK_TOKEN COMMAND_TOKEN SCHEMA_FIELD COUNT_FIELD fields
	{
		fields := append([]Field{$3, $4}, $5...)
		setParsedRecord(cwlex, Record{Kind: OK, Command: $2, Fields: fields})
	}
	| ERR COMMAND_TOKEN CODE_FIELD fields
	{
		fields := append([]Field{$3}, $4...)
		setParsedRecord(cwlex, Record{Kind: Error, Command: $2, Fields: fields})
	}
	| NOTICE_TOKEN COMMAND_TOKEN data_tail
	{
		setParsedRecord(cwlex, Record{
			Kind: Notice, Command: $2, Resource: $3.resource, Fields: $3.fields,
		})
	}
	| EVENT_TOKEN COMMAND_TOKEN data_tail
	{
		setParsedRecord(cwlex, Record{
			Kind: Event, Command: $2, Resource: $3.resource, Fields: $3.fields,
		})
	}
	| ITEM_TOKEN COMMAND_TOKEN optional_tail
	{
		setParsedRecord(cwlex, Record{
			Kind: Item, Command: $2, Resource: $3.resource, Fields: $3.fields,
		})
	}
	| CHUNK_TOKEN COMMAND_TOKEN data_tail
	{
		setParsedRecord(cwlex, Record{
			Kind: Chunk, Command: $2, Resource: $3.resource, Fields: $3.fields,
		})
	}
	;

optional_tail:
	/* empty */
	{
		$$ = parsedTail{}
	}
	| PATH_TOKEN fields
	{
		$$ = parsedTail{resource: $1, fields: $2}
	}
	| nonempty_fields
	{
		$$ = parsedTail{fields: $1}
	}
	;

data_tail:
	nonempty_fields
	{
		$$ = parsedTail{fields: $1}
	}
	| PATH_TOKEN nonempty_fields
	{
		$$ = parsedTail{resource: $1, fields: $2}
	}
	;

fields:
	/* empty */
	{
		$$ = nil
	}
	| nonempty_fields
	{
		$$ = $1
	}
	;

nonempty_fields:
	FIELD_TOKEN
	{
		$$ = []Field{$1}
	}
	| nonempty_fields FIELD_TOKEN
	{
		$$ = append($1, $2)
	}
	;

%%
