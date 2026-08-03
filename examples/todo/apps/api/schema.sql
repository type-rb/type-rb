CREATE TABLE todo_list_entities (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL
);

CREATE TABLE todo_item_entities (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  todo_list_id INTEGER NOT NULL,
  title TEXT NOT NULL,
  completed INTEGER NOT NULL DEFAULT 0,
  FOREIGN KEY (todo_list_id) REFERENCES todo_list_entities(id)
);

CREATE INDEX todo_item_entities_todo_list_id
  ON todo_item_entities(todo_list_id);

CREATE TABLE tag_entities (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE
);

CREATE TABLE todo_item_tag_entities (
  todo_item_id INTEGER NOT NULL,
  tag_id INTEGER NOT NULL,
  PRIMARY KEY (todo_item_id, tag_id),
  FOREIGN KEY (todo_item_id) REFERENCES todo_item_entities(id),
  FOREIGN KEY (tag_id) REFERENCES tag_entities(id)
);

CREATE INDEX todo_item_tag_entities_tag_id
  ON todo_item_tag_entities(tag_id);
