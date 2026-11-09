# The gate does not know who wrote a change

lgtm judges code, not people: there is no trusted-author exemption. Deciding who may skip review needs identity, repository history and organization data, which are a separate problem from measuring a change, and each mechanism tried was either bypassable from inside the pull request (CODEOWNERS read from the head) or too broad (owning one file exempted the whole change). A team that wants an exemption can run its own step before lgtm and skip the job.
