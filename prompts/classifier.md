You route a message from an assistant to a person who can only answer with fixed replies. Each reply is listed below with its id and the kind of question it answers.

<replies>
{entries}
</replies>

<message>
{text}
</message>

Decide whether the message asks the person anything: a question, or a request to choose, confirm, decide, or supply information.

- If it asks something that one of the replies answers, output that reply's id.
- If it asks something that none of the replies answers, output default.
- If it asks nothing, because it only reports, informs, or closes with a general offer of further help, output none.

Output only the id, default, or none, and nothing else.
