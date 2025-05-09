// Constants
const API_BASE = 'http://localhost:16080/ui/v1';

// DOM Elements
const elements = {
    postsList: document.getElementById('posts-list'),
    addPostForm: document.getElementById('add-post-form'),
    postBodyInput: document.getElementById('post-body'),
    postDetail: document.getElementById('post-detail'),
    postContent: document.getElementById('post-content'),
    commentsList: document.getElementById('comments-list'),
    addCommentForm: document.getElementById('add-comment-form'),
    commentBodyInput: document.getElementById('comment-body'),
    pagination: document.getElementById('pagination')
};

// State
let currentPostId = null;
let currentPage = 1;
let totalPages = 1;
const postsPerPage = 10;

// API Functions
const api = {
    async getPosts(page = 1, limit = postsPerPage) {
        const response = await fetch(`${API_BASE}/posts?page=${page}&limit=${limit}`);
        if (!response.ok) throw new Error('Failed to fetch posts');
        return response.json();
    },

    async getPostDetail(id) {
        const response = await fetch(`${API_BASE}/posts/${id}`);
        if (!response.ok) throw new Error('Failed to fetch post details');
        return response.json();
    },

    async createPost(body) {
        const response = await fetch(`${API_BASE}/posts`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ body })
        });
        if (!response.ok) throw new Error('Failed to create post');
        return response.json();
    },

    async createComment(postId, body) {
        const response = await fetch(`${API_BASE}/posts/${postId}/comment`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ body })
        });
        if (!response.ok) throw new Error('Failed to create comment');
        return response.json();
    },

    async deletePost(id) {
        const response = await fetch(`${API_BASE}/posts/${id}`, { method: 'DELETE' });
        if (!response.ok) throw new Error('Failed to delete post');
    }
};

// UI Functions
const ui = {
    createPostElement(post) {
        const div = document.createElement('div');
        div.className = 'post-item';
        const commentCount = post.comment_count || 0;
        div.innerHTML = `
            <div class="post-content">
                <div class="post-body-text">${post.body}</div>
                <div class="post-meta">
                    <span class="comment-count">${commentCount} comments</span>
                    <span class="post-actions">
                        <button class="view" onclick="(() => showPostDetail(${post.id}))()">Comments</button>
                        <button class="delete" onclick="deletePost(${post.id})">Delete</button>
                    </span>
                </div>
            </div>
        `;
        return div;
    },

    updatePostDetail(post) {
        elements.postContent.innerHTML = `
            <div class="post-detail-content">
                <p id="post-body-text">${post.body}</p>
            </div>
        `;

        elements.commentsList.innerHTML = '';
        if (post.comments?.length > 0) {
            post.comments.forEach(comment => {
                const li = document.createElement('li');
                li.className = 'comment-item';
                li.innerHTML = `<p>${comment.body}</p>`;
                elements.commentsList.appendChild(li);
            });
        }
    },

    showModal() {
        elements.postDetail.classList.remove('hidden');
        elements.postDetail.style.display = 'block';
        document.body.classList.add('modal-open');
        document.body.style.overflow = 'hidden';
    },

    hideModal() {
        elements.postDetail.classList.add('hidden');
        document.body.classList.remove('modal-open');
        document.body.style.overflow = '';
        currentPostId = null;
    },

    resetCommentForm() {
        elements.commentBodyInput.value = '';
        elements.addCommentForm.style.display = 'flex';
    },

    showError(message) {
        console.error(message);
        alert(message);
    },

    createPaginationControls() {
        const div = document.createElement('div');
        div.className = 'pagination';
        
        // Previous button
        const prevButton = document.createElement('button');
        prevButton.textContent = '←';
        prevButton.className = currentPage <= 1 ? 'disabled' : '';
        prevButton.onclick = () => {
            if (currentPage > 1) changePage(currentPage - 1);
        };
        
        // Page numbers
        const pageInfo = document.createElement('span');
        pageInfo.textContent = `Page ${currentPage} of ${totalPages}`;
        
        // Next button
        const nextButton = document.createElement('button');
        nextButton.textContent = '→';
        nextButton.className = currentPage >= totalPages ? 'disabled' : '';
        nextButton.onclick = () => {
            if (currentPage < totalPages) changePage(currentPage + 1);
        };
        
        div.appendChild(prevButton);
        div.appendChild(pageInfo);
        div.appendChild(nextButton);
        
        elements.pagination.innerHTML = '';
        elements.pagination.appendChild(div);
    }
};

// Event Handlers
async function fetchPosts() {
    try {
        const response = await api.getPosts(currentPage, postsPerPage);
        elements.postsList.innerHTML = '';
        
        if (Array.isArray(response.posts)) {
            response.posts.forEach(post => {
                elements.postsList.appendChild(ui.createPostElement(post));
            });
        }
        
        // Update pagination
        totalPages = Math.ceil(response.total / postsPerPage);
        ui.createPaginationControls();
    } catch (error) {
        console.error('Error fetching posts:', error);
    }
}

async function showPostDetail(id) {
    try {
        const post = await api.getPostDetail(id);
        currentPostId = id;
        ui.updatePostDetail(post);
        ui.resetCommentForm();
        ui.showModal();
    } catch (error) {
        ui.showError('Failed to load post details');
    }
}

async function deletePost(id) {
    if (!confirm('Delete this post?')) return;
    try {
        await api.deletePost(id);
        await fetchPosts();
        if (currentPostId === id) {
            ui.hideModal();
        }
    } catch (error) {
        ui.showError('Failed to delete post');
    }
}

async function changePage(page) {
    if (page < 1 || page > totalPages) return;
    currentPage = page;
    await fetchPosts();
}

// Event Listeners
document.addEventListener('click', (event) => {
    if (!elements.postDetail.classList.contains('hidden') && 
        !elements.postDetail.contains(event.target) && 
        !event.target.classList.contains('view')) {
        ui.hideModal();
    }
});

elements.postDetail.addEventListener('click', (event) => {
    event.stopPropagation();
});

elements.addPostForm.addEventListener('submit', async (event) => {
    event.preventDefault();
    const body = elements.postBodyInput.value.trim();
    if (!body) return;
    
    try {
        await api.createPost(body);
        elements.postBodyInput.value = '';
        await fetchPosts();
    } catch (error) {
        ui.showError('Failed to create post');
    }
});

elements.addCommentForm.addEventListener('submit', async (event) => {
    event.preventDefault();
    const body = elements.commentBodyInput.value.trim();
    if (!body || !currentPostId) return;
    
    try {
        await api.createComment(currentPostId, body);
        elements.commentBodyInput.value = '';
        await showPostDetail(currentPostId);
        await fetchPosts();
    } catch (error) {
        ui.showError('Failed to add comment');
    }
});

// Make deletePost available globally
window.deletePost = deletePost;

// Initialize
fetchPosts(); 
