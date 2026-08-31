# Database migrations

Store ordered SQL schema changes in this directory.

Naming convention:

```text
000001_initial_schema.up.sql
000001_initial_schema.down.sql
```

Every schema change must be reproducible, reviewed, and accompanied by its
rollback when rollback is safe. The initial migration will be created after
the open decisions in the ERD and API contract are approved.
